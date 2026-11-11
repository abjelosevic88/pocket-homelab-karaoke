package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// ---- what was sung ----

func decodeBoth(r *http.Request, a, b any) error {
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, a); err != nil {
		return err
	}
	return json.Unmarshal(raw, b)
}

func (s *Server) handlePerformances(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := store.PerfFilter{SongID: int64(queryInt(r, "song")), SingerID: int64(queryInt(r, "singer")), NightID: int64(queryInt(r, "night")), Limit: queryInt(r, "limit")}
	if v := r.URL.Query().Get("practice"); v != "" {
		b := v == "1" || v == "true"
		f.Practice = &b
	}
	if d := r.URL.Query().Get("from"); validDate(d) && d != "" {
		f.From = d
	}
	list, err := s.store.ListPerformances(ctx, actorID(ctx), f)
	if err != nil {
		s.fail(w, err, "history")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"performances": list})
}

// handleCreatePerformance keeps a song sung alone on this device (Sing page).
func (s *Server) handleCreatePerformance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		SongID   int64  `json:"song_id"`
		Singer   string `json:"singer"`
		SingerID *int64 `json:"singer_id"`
		Practice bool   `json:"practice"`
		Score    *int   `json:"score"`
		OnTime   *int   `json:"on_time"`
		Steady   *int   `json:"steady"`
		KeyShift int    `json:"key_shift"`
		SungMs   int64  `json:"sung_ms"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	so, err := s.store.GetSong(ctx, u.ID, in.SongID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "that song is not in your library")
		return
	}
	p := &store.Performance{UserID: u.ID, SongID: &so.ID, Title: so.Title, Artist: so.Artist, Singer: cleanText(in.Singer, 80), Practice: in.Practice, KeyShift: clampInt(in.KeyShift, -6, 6),
		SungMs: max(0, in.SungMs), SourceApp: sourceOf(ctx)}
	if in.SingerID != nil && *in.SingerID > 0 {
		if g, err := s.store.GetSinger(ctx, u.ID, *in.SingerID); err == nil {
			p.SingerID = &g.ID
			if p.Singer == "" {
				p.Singer = g.Name
			}
		}
	}
	if p.Singer == "" {
		p.Singer = s.actorName(u)
	}
	clampP := func(v *int) *int {
		if v == nil {
			return nil
		}
		c := clampInt(*v, 0, 100)
		return &c
	}
	p.Score, p.OnTime, p.Steady = clampP(in.Score), clampP(in.OnTime), clampP(in.Steady)
	if err := s.store.CreatePerformance(ctx, p); err != nil {
		s.fail(w, err, "keep")
		return
	}
	payload := map[string]any{"performance_id": p.ID, "title": p.Title, "artist": p.Artist, "singer": p.Singer, "practice": p.Practice}
	if p.Score != nil {
		payload["score"] = *p.Score
	}
	s.publish(ctx, u.ID, "karaoke.performed", fmt.Sprintf("performance:%d", p.ID), payload)
	if p.Practice && p.SungMs > 60000 {
		go s.tickHabit(ctxDetached(ctx), u.ID)
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) perfOf(w http.ResponseWriter, r *http.Request) (*store.Performance, bool) {
	ctx := r.Context()
	p, err := s.store.GetPerformance(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return p, true
}

func (s *Server) handleUpdatePerformance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := s.perfOf(w, r)
	if !ok {
		return
	}
	var in struct {
		Singer   string `json:"singer"`
		SingerID *int64 `json:"singer_id"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["singer"] {
		if v := cleanText(in.Singer, 80); v != "" {
			p.Singer = v
		}
	}
	if keys["singer_id"] {
		p.SingerID = nil
		if in.SingerID != nil && *in.SingerID > 0 {
			g, err := s.store.GetSinger(ctx, p.UserID, *in.SingerID)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "that singer is not there")
				return
			}
			p.SingerID = &g.ID
			if !keys["singer"] {
				p.Singer = g.Name
			}
		}
	}
	if err := s.store.UpdatePerformance(ctx, p); err != nil {
		s.fail(w, err, "history")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePerformance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := s.perfOf(w, r)
	if !ok {
		return
	}
	if err := s.store.DeletePerformance(ctx, p.UserID, p.ID); err != nil {
		s.fail(w, err, "history")
		return
	}
	s.dropMedia(ctx, p.Recording)
	w.WriteHeader(http.StatusNoContent)
}

// handleRecording keeps the recording of a performance (the voice with the song, from the browser).
func (s *Server) handleRecording(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := s.perfOf(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		if p.Recording == "" {
			writeErr(w, http.StatusNotFound, "no recording")
			return
		}
		s.serveMediaFile(w, r, p.Recording, p.RecordingMime, fileSafe(p.Singer+" - "+p.Title)+extForMime(p.RecordingMime))
		return
	}
	mime := strings.Split(r.Header.Get("Content-Type"), ";")[0]
	ext := extForMime(mime)
	if ext == "" {
		writeErr(w, http.StatusBadRequest, "a recording is audio (WebM, Ogg or MP4)")
		return
	}
	rel, _, err := s.saveMedia(p.UserID, ext, r.Body, 300<<20)
	if err != nil {
		s.userFail(w, err, "recording")
		return
	}
	old := p.Recording
	p.Recording, p.RecordingMime = rel, mime
	if err := s.store.UpdatePerformance(ctx, p); err != nil {
		s.fail(w, err, "recording")
		return
	}
	s.dropMedia(ctx, old)
	p.HasRecording = true
	writeJSON(w, http.StatusOK, p)
}

func extForMime(m string) string {
	return map[string]string{"audio/webm": ".weba", "video/webm": ".webm", "audio/ogg": ".ogg", "audio/mp4": ".m4a", "video/mp4": ".mp4", "audio/mpeg": ".mp3", "audio/wav": ".wav"}[m]
}

func fileSafe(v string) string {
	v = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(v))
	return firstNonEmpty(clip(v, 100), "karaoke")
}

// handleStats is the numbers page: most sung songs, singers, a year of singing.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	since := ""
	switch r.URL.Query().Get("period") {
	case "month":
		since = time.Now().AddDate(0, -1, 0).UTC().Format(time.RFC3339Nano)
	case "year":
		since = time.Now().AddDate(-1, 0, 0).UTC().Format(time.RFC3339Nano)
	}
	songs, _ := s.store.TopSongs(ctx, uid, since, 10)
	singers, _ := s.store.TopSingers(ctx, uid, since, 10)
	perDay, _ := s.store.SungPerDay(ctx, uid, time.Now().AddDate(0, 0, -370).UTC().Format("2006-01-02"))
	all, _ := s.store.ListPerformances(ctx, uid, store.PerfFilter{From: since, Limit: 2000})
	total, scored, sum, best := len(all), 0, 0, 0
	var bestP *store.Performance
	var minutes int64
	for i, p := range all {
		minutes += p.SungMs
		if p.Score != nil {
			scored++
			sum += *p.Score
			if *p.Score > best || bestP == nil {
				best, bestP = *p.Score, &all[i]
			}
		}
	}
	avg := 0.0
	if scored > 0 {
		avg = float64(sum) / float64(scored)
	}
	writeJSON(w, http.StatusOK, map[string]any{"songs": songs, "singers": singers, "per_day": perDay, "total": total, "scored": scored, "average": avg, "best": bestP, "minutes": minutes / 60000})
}
