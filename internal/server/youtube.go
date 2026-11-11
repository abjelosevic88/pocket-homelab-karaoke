package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/ytdlp"
)

// YouTube songs play in YouTube's own player (nothing is downloaded), with
// our timed words over it when the video has none. Keeping a copy on this
// server (yt-dlp, when the box has it) makes the song play without YouTube,
// and lets the key change, the voice cut and recording work on it.

var reYouTubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// youTubeID finds the video id in a link (watch, youtu.be, shorts, embed, music) or a bare id.
func youTubeID(v string) string {
	v = strings.TrimSpace(v)
	if reYouTubeID.MatchString(v) {
		return v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	host = strings.TrimPrefix(host, "m.")
	switch host {
	case "youtu.be":
		id := strings.Trim(u.Path, "/")
		if reYouTubeID.MatchString(id) {
			return id
		}
	case "youtube.com", "music.youtube.com", "youtube-nocookie.com":
		if id := u.Query().Get("v"); reYouTubeID.MatchString(id) {
			return id
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live" || parts[0] == "v") && reYouTubeID.MatchString(parts[1]) {
			return parts[1]
		}
	}
	return ""
}

// ytVideo is one search result.
type ytVideo struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Channel    string `json:"channel"`
	DurationMs int64  `json:"duration_ms"`
	Thumb      string `json:"thumb"`
	Karaoke    bool   `json:"karaoke"` // looks like a karaoke video (words on screen, no lead voice)
	Have       int64  `json:"have,omitempty"`
}

func ytThumb(id string) string { return "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg" }

func looksKaraoke(title, channel string) bool {
	t := strings.ToLower(title + " " + channel)
	for _, w := range []string{"karaoke", "instrumental", "sing king", "backing track", "minus", "karaoké", "караоке", "matyas"} {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

var ytSearchCache sync.Map // query → cached search

type ytCached struct {
	at   time.Time
	list []ytVideo
}

// youTubeSearch finds videos: YouTube's own results page first (quick), yt-dlp
// when that page cannot be read.
func (s *Server) youTubeSearch(ctx context.Context, q string) ([]ytVideo, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []ytVideo{}, nil
	}
	if s.cfg.NoLookups {
		return nil, &userErr{"searching YouTube is switched off on this box (LOOKUPS=off)"}
	}
	if c, ok := ytSearchCache.Load(strings.ToLower(q)); ok && time.Since(c.(ytCached).at) < 30*time.Minute {
		return c.(ytCached).list, nil
	}
	list, err := s.youTubePage(ctx, q)
	if err != nil || len(list) == 0 {
		if l2, err2 := s.youTubeByYtdlp(ctx, q); err2 == nil {
			list, err = l2, nil
		} else if err == nil {
			err = err2
		}
	}
	if err != nil {
		return nil, err
	}
	ytSearchCache.Store(strings.ToLower(q), ytCached{at: time.Now(), list: list})
	return list, nil
}

func (s *Server) youTubePage(ctx context.Context, q string) ([]ytVideo, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.youtube.com/results?hl=en&search_query="+url.QueryEscape(q), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(&http.Cookie{Name: "CONSENT", Value: "YES+1"})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YouTube answered HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return parseYouTubePage(string(b)), nil
}

// parseYouTubePage reads the videos out of a results page's ytInitialData.
func parseYouTubePage(page string) []ytVideo {
	out := []ytVideo{}
	i := strings.Index(page, "ytInitialData")
	if i < 0 {
		return out
	}
	j := strings.Index(page[i:], "{")
	if j < 0 {
		return out
	}
	dec := json.NewDecoder(strings.NewReader(page[i+j:]))
	var data any
	if dec.Decode(&data) != nil {
		return out
	}
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		if len(out) >= 20 {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			if vr, ok := t["videoRenderer"].(map[string]any); ok {
				id, _ := vr["videoId"].(string)
				if reYouTubeID.MatchString(id) && !seen[id] {
					seen[id] = true
					title := runsText(vr["title"])
					ch := runsText(vr["ownerText"])
					d := parseClock(simpleText(vr["lengthText"]))
					if d > 0 { // live streams have no length
						out = append(out, ytVideo{ID: id, Title: title, Channel: ch, DurationMs: d, Thumb: ytThumb(id), Karaoke: looksKaraoke(title, ch)})
					}
				}
				return
			}
			for _, x := range t {
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		}
	}
	walk(data)
	return out
}

func runsText(v any) string {
	m, _ := v.(map[string]any)
	if m == nil {
		return ""
	}
	if st, ok := m["simpleText"].(string); ok {
		return st
	}
	runs, _ := m["runs"].([]any)
	var b strings.Builder
	for _, r := range runs {
		if rm, ok := r.(map[string]any); ok {
			t, _ := rm["text"].(string)
			b.WriteString(t)
		}
	}
	return b.String()
}

func simpleText(v any) string { return runsText(v) }

// parseClock reads "3:45" or "1:02:03" as milliseconds.
func parseClock(v string) int64 {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	var total int64
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + int64(n)
	}
	return total * 1000
}

func (s *Server) youTubeByYtdlp(ctx context.Context, q string) ([]ytVideo, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	info, err := s.ytdlp.Inspect(ctx, "ytsearch15:"+q, ytdlp.Options{})
	if err != nil {
		return nil, err
	}
	out := []ytVideo{}
	for _, e := range info.Entries {
		if !reYouTubeID.MatchString(e.ID) {
			continue
		}
		ch := firstNonEmpty(e.Channel, e.Uploader)
		out = append(out, ytVideo{ID: e.ID, Title: e.Title, Channel: ch, DurationMs: int64(e.Duration * 1000), Thumb: ytThumb(e.ID), Karaoke: looksKaraoke(e.Title, ch)})
	}
	return out, nil
}

// youTubeInfo reads a video's title and channel (oEmbed: no key, no download).
func (s *Server) youTubeInfo(ctx context.Context, id string) (ytVideo, error) {
	v := ytVideo{ID: id, Thumb: ytThumb(id)}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.youtube.com/oembed?format=json&url="+url.QueryEscape("https://www.youtube.com/watch?v="+id), nil)
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return v, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return v, &userErr{"YouTube does not know this video (or it may not be played on other sites)"}
	}
	var o struct {
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&o); err != nil {
		return v, err
	}
	v.Title, v.Channel, v.Karaoke = o.Title, o.AuthorName, looksKaraoke(o.Title, o.AuthorName)
	return v, nil
}

// songFromYouTube adds a YouTube video as a song (or returns the one the
// person has already).
func (s *Server) songFromYouTube(ctx context.Context, uid int64, v ytVideo, addedBy string) (*store.Song, bool, error) {
	if old, err := s.store.SongBySource(ctx, uid, "youtube", v.ID); err == nil {
		return old, false, nil
	}
	if v.Title == "" {
		if info, err := s.youTubeInfo(ctx, v.ID); err == nil {
			v.Title, v.Channel, v.Karaoke = info.Title, info.Channel, info.Karaoke
		} else if ue, ok := err.(*userErr); ok {
			return nil, false, ue
		}
	}
	title, artist, k := titleFromFile(v.Title + ".x")
	if artist == "" && !looksKaraoke("", v.Channel) {
		artist = strings.TrimSuffix(v.Channel, " - Topic")
	}
	kind := "original"
	if k || v.Karaoke {
		kind = "karaoke"
	}
	st := s.settingsOf(ctx, uid)
	so := &store.Song{UserID: uid, Title: firstNonEmpty(cleanText(title, maxName), cleanText(v.Title, maxName), "YouTube video"), Artist: cleanText(artist, maxName),
		Kind: kind, Source: "youtube", SourceRef: v.ID, IsVideo: true, CoverURL: ytThumb(v.ID), DurationMs: v.DurationMs, VideoWords: kind == "karaoke",
		AddedBy: cleanText(addedBy, maxName), SourceApp: sourceOf(ctx)}
	if kind == "original" {
		so.VoiceCut = st.DefaultVoice
	}
	if st.KeepCopies && s.canKeepCopies() {
		so.CopyState = "waiting"
	}
	if err := s.store.CreateSong(ctx, so); err != nil {
		return nil, false, err
	}
	return so, true, nil
}

// ---- keeping a copy (yt-dlp) ----

func (s *Server) canKeepCopies() bool { return s.ytdlp != nil && !s.cfg.NoDownloads }

// copier keeps copies of YouTube songs one at a time.
func (s *Server) copier(ctx context.Context) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	// a copy cut off by a restart starts again
	if list, err := s.store.SongsWithCopyState(ctx, "working"); err == nil {
		for _, so := range list {
			_ = s.store.SetCopyProgress(ctx, so.ID, "waiting", 0, "")
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.copyKick:
		}
		list, err := s.store.SongsWithCopyState(ctx, "waiting")
		if err != nil || len(list) == 0 {
			continue
		}
		s.keepCopy(ctx, &list[0])
	}
}

func (s *Server) kickCopier() {
	select {
	case s.copyKick <- struct{}{}:
	default:
	}
}

func (s *Server) keepCopy(ctx context.Context, so *store.Song) {
	if !s.canKeepCopies() || so.Source != "youtube" {
		_ = s.store.SetCopyProgress(ctx, so.ID, "failed", 0, "keeping copies is not possible on this box (no yt-dlp)")
		return
	}
	_ = s.store.SetCopyProgress(ctx, so.ID, "working", 0, "")
	st := s.settingsOf(ctx, so.UserID)
	tmp := filepath.Join(s.cfg.DataDir, "tmp", "yt-"+randomHex(6))
	defer os.RemoveAll(tmp)
	jctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	last := time.Time{}
	res, err := s.ytdlp.Download(jctx, ytdlp.Job{URL: "https://www.youtube.com/watch?v=" + so.SourceRef, Dir: tmp, TempDir: tmp + "/parts", Name: "song", Video: so.IsVideo, Height: st.VideoQuality}, func(p ytdlp.Progress) {
		if time.Since(last) < 2*time.Second {
			return
		}
		last = time.Now()
		total := float64(p.Total)
		if total == 0 {
			total = p.Estimate
		}
		pc := 0
		if total > 0 {
			pc = int(float64(p.Downloaded) * 100 / total)
		}
		_ = s.store.SetCopyProgress(ctx, so.ID, "working", min(pc, 99), "")
	})
	cur, gerr := s.store.GetSong(ctx, so.UserID, so.ID)
	if gerr != nil { // the song went away meanwhile
		return
	}
	if err != nil {
		_ = s.store.SetCopyProgress(ctx, so.ID, "failed", 0, clip(err.Error(), 300))
		if st.NotifyCopy {
			s.push(ctx, so.UserID, st, "A YouTube copy failed", cur.Title+": "+clip(err.Error(), 200), fmt.Sprintf("/songs/%d", so.ID))
		}
		return
	}
	rel, n, err := s.moveIntoMedia(so.UserID, res.File)
	if err != nil {
		_ = s.store.SetCopyProgress(ctx, so.ID, "failed", 0, clip(err.Error(), 300))
		return
	}
	cur.Media, cur.MediaSize, cur.MediaMime = rel, n, mimeByExt(rel)
	cur.CopyState, cur.CopyProgress, cur.CopyError = "ready", 100, ""
	if pi := s.probe(ctx, s.mediaPath(rel)); pi.DurationMs > 0 {
		cur.DurationMs = pi.DurationMs
		cur.IsVideo = pi.Video
	}
	if res.Thumb != "" && cur.Cover == "" {
		if r2, _, err := s.moveIntoMedia(so.UserID, res.Thumb); err == nil {
			cur.Cover = r2
		}
	}
	if err := s.store.UpdateSong(ctx, cur); err != nil {
		s.dropMedia(ctx, rel)
		return
	}
	if st.NotifyCopy {
		s.push(ctx, so.UserID, st, "A YouTube song is kept", cur.Title+" now plays from this server, with key change and voice cut.", fmt.Sprintf("/songs/%d", so.ID))
	}
}
