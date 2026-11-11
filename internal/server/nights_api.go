package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// ---- nights ----

const codeLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O, 1/I

func newJoinCode() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = codeLetters[int(b[i])%len(codeLetters)]
	}
	return string(b)
}

func (s *Server) nightOf(w http.ResponseWriter, r *http.Request) (*store.Night, bool) {
	ctx := r.Context()
	n, err := s.store.GetNight(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return n, true
}

func (s *Server) handleNights(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListNights(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "nights")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nights": list})
}

func (s *Server) handleGetNight(w http.ResponseWriter, r *http.Request) {
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	host, _, _ := s.states(ctx, n)
	guests, _ := s.store.ListGuests(ctx, n.UserID, n.ID)
	perfs, _ := s.store.ListPerformances(ctx, n.UserID, store.PerfFilter{NightID: n.ID, Limit: 500})
	writeJSON(w, http.StatusOK, map[string]any{"night": n, "live": host, "guests": guests, "performances": perfs,
		"join_url": s.joinURL(n.JoinCode), "screen_url": s.link("/screen/" + n.ScreenKey)})
}

type nightIn struct {
	Title        string `json:"title"`
	StartsAt     string `json:"starts_at"`
	Place        string `json:"place"`
	About        string `json:"about"`
	Rotation     string `json:"rotation"`
	Approve      bool   `json:"approve"`
	GuestYouTube bool   `json:"guest_youtube"`
	PerGuest     int    `json:"per_guest"`
	BookID       *int64 `json:"book_id"`
	Scores       bool   `json:"scores"`
	Autoplay     int    `json:"autoplay"`
	Calendar     bool   `json:"calendar"`
}

func validStart(v string) bool {
	if v == "" {
		return true
	}
	for _, f := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02"} {
		if _, err := time.Parse(f, v); err == nil {
			return true
		}
	}
	return false
}

func (s *Server) applyNight(ctx context.Context, uid int64, n *store.Night, in nightIn, keys map[string]bool) error {
	if keys["title"] || n.ID == 0 {
		n.Title = cleanText(in.Title, maxName)
		if n.Title == "" {
			n.Title = "Karaoke night"
		}
	}
	if keys["starts_at"] {
		v := strings.TrimSpace(in.StartsAt)
		if len(v) > 16 {
			v = v[:16]
		}
		if !validStart(v) {
			return &userErr{"the start is a date and a time"}
		}
		n.StartsAt = v
	}
	if keys["place"] {
		n.Place = cleanText(in.Place, maxLine)
	}
	if keys["about"] {
		n.About = cleanText(in.About, 4000)
	}
	if keys["rotation"] {
		if in.Rotation != "fair" && in.Rotation != "order" {
			return &userErr{"the queue goes round fairly or in the order asked"}
		}
		n.Rotation = in.Rotation
	}
	if keys["approve"] {
		n.Approve = in.Approve
	}
	if keys["guest_youtube"] {
		n.GuestYouTube = in.GuestYouTube
	}
	if keys["per_guest"] {
		n.PerGuest = clampInt(in.PerGuest, 0, 10)
	}
	if keys["book_id"] {
		n.BookID = nil
		if in.BookID != nil && *in.BookID > 0 {
			if _, err := s.store.GetBook(ctx, uid, *in.BookID); err != nil {
				return &userErr{"that song book is not there"}
			}
			n.BookID = in.BookID
		}
	}
	if keys["scores"] {
		n.Scores = in.Scores
	}
	if keys["autoplay"] {
		n.Autoplay = clampInt(in.Autoplay, 0, 120)
	}
	return nil
}

func (s *Server) handleCreateNight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in nightIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st := s.settingsOf(ctx, u.ID)
	n := &store.Night{UserID: u.ID, Rotation: st.Rotation, Approve: st.Approve, GuestYouTube: st.GuestYouTube, PerGuest: st.PerGuest, Scores: st.Scores, Autoplay: st.Autoplay,
		JoinCode: newJoinCode(), ScreenKey: newToken() + newToken()[:8], SourceApp: sourceOf(ctx)}
	if err := s.applyNight(ctx, u.ID, n, in, keys); err != nil {
		s.userFail(w, err, "night")
		return
	}
	for i := 0; ; i++ {
		err = s.store.CreateNight(ctx, n)
		if err == store.ErrConflict && i < 5 {
			n.JoinCode = newJoinCode()
			continue
		}
		break
	}
	if err != nil {
		s.fail(w, err, "night")
		return
	}
	s.publish(ctx, u.ID, "karaoke.night_planned", fmt.Sprintf("night:%d", n.ID), map[string]any{"night_id": n.ID, "title": n.Title, "starts_at": n.StartsAt, "place": n.Place, "url": s.link(fmt.Sprintf("/nights/%d", n.ID))})
	calendar := st.CalendarOnNight
	if keys["calendar"] {
		calendar = in.Calendar
	}
	if calendar && n.StartsAt != "" && sourceOf(ctx) == "" {
		if ref, err := s.nightToCalendar(ctx, u.ID, n); err == nil && ref != "" {
			n.CalendarRef = ref
			_ = s.store.UpdateNight(ctx, n)
		}
	}
	n, _ = s.store.GetNight(ctx, u.ID, n.ID)
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleUpdateNight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	if !mayChange(ctx, n.SourceApp) {
		writeErr(w, http.StatusForbidden, "this app may change only the nights it made")
		return
	}
	var in nightIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.applyNight(ctx, n.UserID, n, in, keys); err != nil {
		s.userFail(w, err, "night")
		return
	}
	if queryBool(r, "new_code") {
		n.JoinCode = newJoinCode()
	}
	if queryBool(r, "new_screen") {
		n.ScreenKey = newToken() + newToken()[:8]
		b := []byte(`{"type":"gone"}`)
		s.live.send(n.ID, nil, b, nil)
	}
	if err := s.store.UpdateNight(ctx, n); err != nil {
		s.fail(w, err, "night")
		return
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	n, _ = s.store.GetNight(ctx, n.UserID, n.ID)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteNight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	if !mayChange(ctx, n.SourceApp) {
		writeErr(w, http.StatusForbidden, "this app may remove only the nights it made")
		return
	}
	if err := s.store.DeleteNight(ctx, n.UserID, n.ID); err != nil {
		s.fail(w, err, "night")
		return
	}
	b := []byte(`{"type":"gone"}`)
	s.live.send(n.ID, b, b, b)
	w.WriteHeader(http.StatusNoContent)
}

// ---- the queue ----

// singerKey says who an entry is for, to go round fairly.
func singerKey(e store.Entry) string {
	switch {
	case e.GuestID != nil:
		return fmt.Sprintf("g%d", *e.GuestID)
	case e.SingerID != nil:
		return fmt.Sprintf("s%d", *e.SingerID)
	}
	return "n" + strings.ToLower(strings.TrimSpace(e.Singer))
}

// placeFor is where a new entry goes in the waiting list (0-based). Going
// round fairly, each singer gets one song per round: someone asking for a
// third song waits until everyone has sung twice.
func placeFor(rotation string, waiting []store.Entry, singing *store.Entry, add store.Entry) int {
	if rotation != "fair" {
		return len(waiting)
	}
	count := map[string]int{}
	if singing != nil {
		count[singerKey(*singing)] = 1 // singing now has had this round's song
	}
	round := make([]int, len(waiting))
	for i, e := range waiting {
		count[singerKey(e)]++
		round[i] = count[singerKey(e)]
	}
	mine := count[singerKey(add)] + 1
	at := 0
	for i := range waiting {
		if round[i] <= mine {
			at = i + 1
		}
	}
	return at
}

// enqueue adds an entry to a night's queue at its place and renumbers the waiting list.
func (s *Server) enqueue(ctx context.Context, n *store.Night, e *store.Entry, front bool) error {
	q, err := s.store.Queue(ctx, n.ID)
	if err != nil {
		return err
	}
	var waiting []store.Entry
	var singing *store.Entry
	for i := range q {
		switch q[i].Status {
		case "waiting":
			waiting = append(waiting, q[i])
		case "singing":
			singing = &q[i]
		}
	}
	at := len(waiting)
	if e.Status == "asked" {
		at = len(waiting) // not in line until the host says yes
	} else if front {
		at = 0
	} else {
		at = placeFor(n.Rotation, waiting, singing, *e)
	}
	e.Position = at + 1
	if e.Status == "asked" {
		e.Position = 1000 + len(q)
	}
	if err := s.store.AddEntry(ctx, e); err != nil {
		return err
	}
	if e.Status == "waiting" {
		ids := make([]int64, 0, len(waiting)+1)
		for i, w := range waiting {
			if i == at {
				ids = append(ids, e.ID)
			}
			ids = append(ids, w.ID)
		}
		if at >= len(waiting) {
			ids = append(ids, e.ID)
		}
		_ = s.store.Reorder(ctx, n.UserID, n.ID, ids)
	}
	return nil
}

type entryIn struct {
	SongID   int64  `json:"song_id"`
	Singer   string `json:"singer"`
	SingerID *int64 `json:"singer_id"`
	GuestID  *int64 `json:"guest_id"`
	KeyShift int    `json:"key_shift"`
	Note     string `json:"note"`
	Front    bool   `json:"front"` // straight to the top of the queue
	Status   string `json:"status"`
}

func (s *Server) handleAddEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	if n.Status == "ended" {
		writeErr(w, http.StatusBadRequest, "this night is over")
		return
	}
	var in entryIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	so, err := s.store.GetSong(ctx, n.UserID, in.SongID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "that song is not in your library")
		return
	}
	e := &store.Entry{UserID: n.UserID, NightID: n.ID, SongID: so.ID, Singer: cleanText(in.Singer, 80), KeyShift: clampInt(in.KeyShift, -6, 6), Note: cleanText(in.Note, 200)}
	if in.SingerID != nil && *in.SingerID > 0 {
		g, err := s.store.GetSinger(ctx, n.UserID, *in.SingerID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "that singer is not there")
			return
		}
		e.SingerID = &g.ID
		if e.Singer == "" {
			e.Singer = g.Name
		}
	}
	if in.GuestID != nil && *in.GuestID > 0 {
		g, err := s.store.GetGuest(ctx, n.UserID, *in.GuestID)
		if err != nil || g.NightID != n.ID {
			writeErr(w, http.StatusBadRequest, "that guest is not at this night")
			return
		}
		e.GuestID = &g.ID
		if e.Singer == "" {
			e.Singer = g.Name
		}
	}
	if e.Singer == "" {
		e.Singer = s.actorName(userFrom(ctx))
	}
	if e.KeyShift == 0 {
		e.KeyShift = so.KeyShift
	}
	if err := s.enqueue(ctx, n, e, in.Front); err != nil {
		s.fail(w, err, "queue")
		return
	}
	if x, err := s.store.GetEntry(ctx, n.UserID, e.ID); err == nil {
		e = x
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) entryOf(w http.ResponseWriter, r *http.Request) (*store.Entry, *store.Night, bool) {
	ctx := r.Context()
	e, err := s.store.GetEntry(ctx, actorID(ctx), idParam(r, "eid"))
	if err != nil || e.NightID != idParam(r, "id") {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	n, err := s.store.GetNight(ctx, actorID(ctx), e.NightID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return e, n, true
}

// handleUpdateEntry changes an entry: singer, key, a yes to an asked song, back in line.
func (s *Server) handleUpdateEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e, n, ok := s.entryOf(w, r)
	if !ok {
		return
	}
	var in entryIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["singer"] {
		if v := cleanText(in.Singer, 80); v != "" {
			e.Singer = v
		}
	}
	if keys["key_shift"] {
		e.KeyShift = clampInt(in.KeyShift, -6, 6)
		if e.Status == "singing" {
			n.KeyNow = e.KeyShift
			_ = s.store.UpdateNight(ctx, n)
		}
	}
	if keys["note"] {
		e.Note = cleanText(in.Note, 200)
	}
	if keys["song_id"] && in.SongID != e.SongID && e.Status != "singing" {
		if _, err := s.store.GetSong(ctx, n.UserID, in.SongID); err != nil {
			writeErr(w, http.StatusBadRequest, "that song is not in your library")
			return
		}
		e.SongID = in.SongID
	}
	requeue := false
	if keys["status"] {
		switch {
		case in.Status == "waiting" && (e.Status == "asked" || e.Status == "skipped" || e.Status == "done"):
			requeue = true
		case in.Status == "skipped" && (e.Status == "waiting" || e.Status == "asked"):
			at := store.Now()
			e.Status, e.DoneAt = "skipped", &at
		}
	}
	if requeue {
		if err := s.store.DeleteEntry(ctx, n.UserID, e.ID); err != nil {
			s.fail(w, err, "queue")
			return
		}
		ne := &store.Entry{UserID: e.UserID, NightID: e.NightID, SongID: e.SongID, Singer: e.Singer, SingerID: e.SingerID, GuestID: e.GuestID, KeyShift: e.KeyShift, Note: e.Note, Status: "waiting", CreatedAt: e.CreatedAt}
		if err := s.enqueue(ctx, n, ne, in.Front); err != nil {
			s.fail(w, err, "queue")
			return
		}
		e = ne
	} else if err := s.store.UpdateEntry(ctx, e); err != nil {
		s.fail(w, err, "queue")
		return
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e, n, ok := s.entryOf(w, r)
	if !ok {
		return
	}
	if e.Status == "singing" {
		writeErr(w, http.StatusBadRequest, "this song is on now: skip it first")
		return
	}
	if err := s.store.DeleteEntry(ctx, n.UserID, e.ID); err != nil {
		s.fail(w, err, "queue")
		return
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReorder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil || len(in.IDs) > 1000 {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.Reorder(ctx, n.UserID, n.ID, in.IDs); err != nil {
		s.fail(w, err, "queue")
		return
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	w.WriteHeader(http.StatusNoContent)
}

// ---- running the night ----

type controlIn struct {
	Action string `json:"action"` // start | end | next | play | pause | restart | skip | seek | key | voice | volume | stop
	Value  int64  `json:"value"`
	// what the big screen measured when a song ends
	Score  *int  `json:"score"`
	OnTime *int  `json:"on_time"`
	Steady *int  `json:"steady"`
	SungMs int64 `json:"sung_ms"`
	Entry  int64 `json:"entry_id"`
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	var in controlIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.control(ctx, n, in, true); err != nil {
		s.userFail(w, err, "control")
		return
	}
	n, _ = s.store.GetNight(ctx, n.UserID, n.ID)
	host, _, _ := s.states(ctx, n)
	writeJSON(w, http.StatusOK, map[string]any{"night": n, "live": host})
}

func (s *Server) setPhase(n *store.Night, phase string) {
	at := store.Now()
	n.Phase, n.PhaseAt = phase, &at
}

// control moves a night on. host = the press came from the host (not the big screen).
func (s *Server) control(ctx context.Context, n *store.Night, in controlIn, host bool) error {
	switch in.Action {
	case "start":
		if n.Status == "ended" && !host {
			return &userErr{"this night is over"}
		}
		if n.Status != "live" {
			at := store.Now()
			n.Status, n.StartedAt, n.EndedAt = "live", &at, nil
			if n.Phase == "" {
				n.Phase = "idle"
			}
			s.publish(ctx, n.UserID, "karaoke.night_started", fmt.Sprintf("night:%d", n.ID), map[string]any{"night_id": n.ID, "title": n.Title, "join_url": s.joinURL(n.JoinCode)})
		}
	case "end":
		if n.Status != "live" {
			return &userErr{"this night is not on"}
		}
		if cur := s.currentEntry(ctx, n); cur != nil {
			s.finishEntry(ctx, n, cur, controlIn{}, true)
		}
		at := store.Now()
		n.Status, n.EndedAt, n.CurrentID, n.Phase = "ended", &at, nil, "idle"
		if err := s.store.UpdateNight(ctx, n); err != nil {
			return err
		}
		go s.afterNight(context.WithoutCancel(ctx), n.UserID, n.ID)
		go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
		return nil
	case "next":
		if n.Status != "live" {
			return &userErr{"start the night first"}
		}
		if cur := s.currentEntry(ctx, n); cur != nil {
			if cur.Status == "singing" {
				s.finishEntry(ctx, n, cur, controlIn{}, true)
			}
		}
		if !s.callNext(ctx, n) {
			n.CurrentID = nil
			s.setPhase(n, "idle")
		}
	case "play":
		if n.Phase == "next" || n.Phase == "paused" || n.Phase == "singing" {
			s.setPhase(n, "singing")
			s.live.command(n.ID, "play", 0)
		}
	case "pause":
		if n.Phase == "singing" {
			s.setPhase(n, "paused")
			s.live.command(n.ID, "pause", 0)
		}
	case "restart":
		s.live.command(n.ID, "seek", 0)
	case "seek":
		s.live.command(n.ID, "seek", max(0, in.Value))
	case "skip":
		cur := s.currentEntry(ctx, n)
		if cur != nil && cur.Status == "singing" {
			at := store.Now()
			cur.Status, cur.DoneAt = "skipped", &at
			_ = s.store.UpdateEntry(ctx, cur)
		}
		s.live.command(n.ID, "stop", 0)
		if !s.callNext(ctx, n) {
			n.CurrentID = nil
			s.setPhase(n, "idle")
		}
	case "stop":
		s.live.command(n.ID, "stop", 0)
		if cur := s.currentEntry(ctx, n); cur != nil && cur.Status == "singing" {
			cur.Status, cur.StartedAt = "waiting", nil
			_ = s.store.UpdateEntry(ctx, cur)
			_ = s.enqueueBack(ctx, n, cur)
		}
		n.CurrentID = nil
		s.setPhase(n, "idle")
	case "key":
		n.KeyNow = clampInt(int(in.Value), -6, 6)
		if cur := s.currentEntry(ctx, n); cur != nil {
			cur.KeyShift = n.KeyNow
			_ = s.store.UpdateEntry(ctx, cur)
		}
	case "voice":
		n.VoiceNow = clampInt(int(in.Value), 0, 100)
	case "volume":
		n.Volume = clampInt(int(in.Value), 0, 100)
	case "finished": // the big screen: the song came to its end
		cur := s.currentEntry(ctx, n)
		if cur == nil || (in.Entry > 0 && cur.ID != in.Entry) || cur.Status != "singing" {
			return nil // said twice, or already moved on
		}
		s.finishEntry(ctx, n, cur, in, false)
		s.setPhase(n, "score")
		if !n.Scores {
			if !s.callNext(ctx, n) {
				n.CurrentID = nil
				s.setPhase(n, "idle")
			}
		}
	case "advance": // the big screen: the score has been shown
		if n.Phase != "score" {
			return nil
		}
		if !s.callNext(ctx, n) {
			n.CurrentID = nil
			s.setPhase(n, "idle")
		}
	case "started": // the big screen: the song is playing
		if n.Phase == "next" || n.Phase == "paused" {
			s.setPhase(n, "singing")
		}
	default:
		return &userErr{"unknown action"}
	}
	if err := s.store.UpdateNight(ctx, n); err != nil {
		return err
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	return nil
}

func (s *Server) currentEntry(ctx context.Context, n *store.Night) *store.Entry {
	if n.CurrentID == nil {
		return nil
	}
	e, err := s.store.GetEntry(ctx, n.UserID, *n.CurrentID)
	if err != nil {
		return nil
	}
	return e
}

// enqueueBack puts a stopped entry back at the top of the line.
func (s *Server) enqueueBack(ctx context.Context, n *store.Night, e *store.Entry) error {
	q, err := s.store.Queue(ctx, n.ID)
	if err != nil {
		return err
	}
	ids := []int64{e.ID}
	for _, x := range q {
		if x.Status == "waiting" && x.ID != e.ID {
			ids = append(ids, x.ID)
		}
	}
	return s.store.Reorder(ctx, n.UserID, n.ID, ids)
}

// callNext puts the first waiting entry on the screen ("up next").
func (s *Server) callNext(ctx context.Context, n *store.Night) bool {
	q, err := s.store.Queue(ctx, n.ID)
	if err != nil {
		return false
	}
	for _, e := range q {
		if e.Status != "waiting" {
			continue
		}
		at := store.Now()
		e.Status, e.StartedAt = "singing", &at
		if err := s.store.UpdateEntry(ctx, &e); err != nil {
			return false
		}
		id := e.ID
		n.CurrentID = &id
		n.KeyNow, n.PositionMs = e.KeyShift, 0
		n.VoiceNow = 0
		if so, err := s.store.GetSong(ctx, n.UserID, e.SongID); err == nil {
			n.VoiceNow = so.VoiceCut
		}
		s.setPhase(n, "next")
		return true
	}
	return false
}

// finishEntry marks an entry sung and keeps the performance with its score.
func (s *Server) finishEntry(ctx context.Context, n *store.Night, e *store.Entry, in controlIn, cut bool) {
	at := store.Now()
	e.Status, e.DoneAt = "done", &at
	if cut && in.SungMs == 0 {
		if e.StartedAt != nil {
			if t, err := time.Parse(time.RFC3339Nano, *e.StartedAt); err == nil && time.Since(t) < 15*time.Second {
				e.Status = "skipped" // stopped before it really began
			}
		}
	}
	_ = s.store.UpdateEntry(ctx, e)
	if e.Status != "done" {
		return
	}
	so, _ := s.store.GetSong(ctx, n.UserID, e.SongID)
	p := &store.Performance{UserID: n.UserID, NightID: &n.ID, QueueID: &e.ID, Singer: e.Singer, SingerID: e.SingerID, GuestID: e.GuestID, KeyShift: e.KeyShift, SungMs: in.SungMs, Cheers: s.live.cheersOf(e.ID)}
	if so != nil {
		p.SongID, p.Title, p.Artist = &so.ID, so.Title, so.Artist
	} else {
		p.Title, p.Artist = e.Title, e.Artist
	}
	if n.Scores && in.Score != nil {
		sc := clampInt(*in.Score, 0, 100)
		p.Score = &sc
		if in.OnTime != nil {
			v := clampInt(*in.OnTime, 0, 100)
			p.OnTime = &v
		}
		if in.Steady != nil {
			v := clampInt(*in.Steady, 0, 100)
			p.Steady = &v
		}
	}
	if avg, nv := s.live.votesOf(e.ID); nv > 0 {
		p.Stars, p.Votes = &avg, nv
	}
	if err := s.store.CreatePerformance(ctx, p); err != nil {
		s.log.Warn("keep performance", "err", err)
		return
	}
	payload := map[string]any{"night_id": n.ID, "performance_id": p.ID, "title": p.Title, "artist": p.Artist, "singer": p.Singer, "cheers": p.Cheers}
	if p.Score != nil {
		payload["score"] = *p.Score
	}
	s.publish(ctx, n.UserID, "karaoke.performed", fmt.Sprintf("performance:%d", p.ID), payload)
}

// a vote that comes after the score was kept still counts
func (s *Server) updateVotes(ctx context.Context, n *store.Night, entryID int64) {
	p, err := s.store.PerformanceByQueue(ctx, n.UserID, entryID)
	if err != nil {
		return
	}
	if avg, nv := s.live.votesOf(entryID); nv > 0 {
		p.Stars, p.Votes = &avg, nv
	}
	p.Cheers = max(p.Cheers, s.live.cheersOf(entryID))
	_ = s.store.UpdatePerformance(ctx, p)
}

func (s *Server) handleLiveStream(w http.ResponseWriter, r *http.Request) {
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	s.stream(w, r, n, roleHost, 0)
}

// handleGuests lists who joined; PUT renames, links to a singer or blocks one.
func (s *Server) handleGuests(w http.ResponseWriter, r *http.Request) {
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	list, err := s.store.ListGuests(r.Context(), n.UserID, n.ID)
	if err != nil {
		s.fail(w, err, "guests")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"guests": list})
}

func (s *Server) handleUpdateGuest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	g, err := s.store.GetGuest(ctx, n.UserID, idParam(r, "gid"))
	if err != nil || g.NightID != n.ID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var in struct {
		Name     string `json:"name"`
		SingerID *int64 `json:"singer_id"`
		Blocked  bool   `json:"blocked"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["name"] {
		if v := cleanText(in.Name, 40); v != "" {
			g.Name = v
		}
	}
	if keys["singer_id"] {
		g.SingerID = nil
		if in.SingerID != nil && *in.SingerID > 0 {
			if _, err := s.store.GetSinger(ctx, n.UserID, *in.SingerID); err != nil {
				writeErr(w, http.StatusBadRequest, "that singer is not there")
				return
			}
			g.SingerID = in.SingerID
		}
	}
	if keys["blocked"] {
		g.Blocked = in.Blocked
	}
	if err := s.store.UpdateGuest(ctx, g); err != nil {
		s.fail(w, err, "guest")
		return
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	writeJSON(w, http.StatusOK, g)
}

// afterNight runs when a night ends: the results to the bus, a Pocket Music
// playlist of what was sung when the person wants one.
func (s *Server) afterNight(ctx context.Context, uid, nightID int64) {
	n, err := s.store.GetNight(ctx, uid, nightID)
	if err != nil {
		return
	}
	perfs, _ := s.store.ListPerformances(ctx, uid, store.PerfFilter{NightID: n.ID, Limit: 500})
	best := ""
	bestScore := -1
	for _, p := range perfs {
		if p.Score != nil && *p.Score > bestScore {
			bestScore, best = *p.Score, p.Singer
		}
	}
	payload := map[string]any{"night_id": n.ID, "title": n.Title, "songs": len(perfs), "guests": n.Guests, "url": s.link(fmt.Sprintf("/nights/%d", n.ID))}
	if best != "" {
		payload["best_singer"], payload["best_score"] = best, bestScore
	}
	s.publish(ctx, uid, "karaoke.night_ended", fmt.Sprintf("night:%d", n.ID), payload)
	if st := s.settingsOf(ctx, uid); st.MusicPlaylist && len(perfs) > 0 {
		if _, err := s.nightPlaylist(ctx, uid, n, perfs); err != nil {
			s.log.Debug("night playlist", "err", err)
		}
	}
}
