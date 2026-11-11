package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// A live night. Every screen following it — the big screen playing the songs,
// the host's page, each guest's phone — keeps an event stream open; a change
// anywhere (a song asked for, a press on the host's phone, the song ending on
// the big screen) reaches every screen at once.
//
// Guests see only first names in the queue and the songs' titles; never the
// host's library beyond the night's song book, never anyone's score history.

type liveRole int

const (
	roleHost liveRole = iota
	roleScreen
	roleGuest
)

type liveSub struct {
	ch    chan []byte
	role  liveRole
	guest int64
}

type liveHub struct {
	mu     sync.Mutex
	subs   map[int64]map[*liveSub]bool // by night
	votes  map[string]int              // "entry:guest" → stars
	cheers map[int64]int               // entry → reactions
	reacts map[string]time.Time        // "night:who" → last reaction
	pos    map[int64]int64             // night → last position the screen reported
}

func newLiveHub() *liveHub {
	return &liveHub{subs: map[int64]map[*liveSub]bool{}, votes: map[string]int{}, cheers: map[int64]int{}, reacts: map[string]time.Time{}, pos: map[int64]int64{}}
}

func (h *liveHub) sub(night int64, role liveRole, guest int64) *liveSub {
	h.mu.Lock()
	defer h.mu.Unlock()
	sb := &liveSub{ch: make(chan []byte, 32), role: role, guest: guest}
	if h.subs[night] == nil {
		h.subs[night] = map[*liveSub]bool{}
	}
	h.subs[night][sb] = true
	return sb
}

func (h *liveHub) unsub(night int64, sb *liveSub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[night], sb)
	if len(h.subs[night]) == 0 {
		delete(h.subs, night)
	}
}

// counts says how many screens and phones follow a night.
func (h *liveHub) counts(night int64) (screens, phones int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sb := range h.subs[night] {
		switch sb.role {
		case roleScreen:
			screens++
		case roleGuest:
			phones++
		}
	}
	return
}

// send gives each role its own message (nil = not for that role).
func (h *liveHub) send(night int64, host, screen, guest []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sb := range h.subs[night] {
		b := host
		switch sb.role {
		case roleScreen:
			b = screen
		case roleGuest:
			b = guest
		}
		if b == nil {
			continue
		}
		select {
		case sb.ch <- b:
		default:
		}
	}
}

func (h *liveHub) addCheer(entry int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cheers[entry]++
	return h.cheers[entry]
}

func (h *liveHub) cheersOf(entry int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cheers[entry]
}

func (h *liveHub) vote(entry, guest int64, stars int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.votes[fmt.Sprintf("%d:%d", entry, guest)] = stars
}

// votesOf is the average of the room's stars for an entry and how many voted.
func (h *liveHub) votesOf(entry int64) (float64, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	prefix := fmt.Sprintf("%d:", entry)
	sum, n := 0, 0
	for k, v := range h.votes {
		if strings.HasPrefix(k, prefix) {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return float64(sum) / float64(n), n
}

func (h *liveHub) myVote(entry, guest int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.votes[fmt.Sprintf("%d:%d", entry, guest)]
}

// songChanged reloads the screens of the person's live nights showing a song.
func (h *liveHub) songChanged(ctx context.Context, s *Server, uid, songID int64) {
	nights, err := s.store.ListNights(ctx, uid)
	if err != nil {
		return
	}
	for _, n := range nights {
		if n.Status == "live" {
			go h.broadcast(context.WithoutCancel(ctx), s, n.ID)
		}
	}
}

// ---- what each screen gets ----

// liveSong is a song as the big screen plays it.
type liveSong struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Language   string `json:"language"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	YouTube    string `json:"youtube,omitempty"` // play this video in YouTube's player
	Media      string `json:"media,omitempty"`   // or this file
	MediaMime  string `json:"media_mime,omitempty"`
	IsVideo    bool   `json:"is_video"`
	CDG        string `json:"cdg,omitempty"`
	Cover      string `json:"cover,omitempty"`
	Lyrics     string `json:"lyrics"`
	Synced     bool   `json:"synced"`
	OffsetMs   int64  `json:"offset_ms"`
	VideoWords bool   `json:"video_words"`
	DurationMs int64  `json:"duration_ms"`
	VoiceCut   int    `json:"voice_cut"`
	KeyShift   int    `json:"key_shift"`
}

func screenSong(so *store.Song, base string) *liveSong {
	ls := &liveSong{ID: so.ID, Title: so.Title, Artist: so.Artist, Language: so.Language, Kind: so.Kind, Source: so.Source, IsVideo: so.IsVideo, Lyrics: so.Lyrics, Synced: so.LyricsSynced,
		OffsetMs: so.LyricsOffsetMs, VideoWords: so.VideoWords, DurationMs: so.DurationMs, VoiceCut: so.VoiceCut, KeyShift: so.KeyShift, MediaMime: so.MediaMime}
	if so.Media != "" || (so.Source == "music" && so.SourceRef != "") {
		ls.Media = fmt.Sprintf("%s/songs/%d/media", base, so.ID)
	} else if so.Source == "youtube" {
		ls.YouTube = so.SourceRef
	}
	if so.CDG != "" {
		ls.CDG = fmt.Sprintf("%s/songs/%d/cdg", base, so.ID)
	}
	if so.Cover != "" || so.CoverURL != "" || so.Source == "music" {
		ls.Cover = fmt.Sprintf("%s/songs/%d/cover", base, so.ID)
	}
	return ls
}

// liveEntry is a queue entry as the screens show it.
type liveEntry struct {
	ID       int64  `json:"id"`
	SongID   int64  `json:"song_id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Singer   string `json:"singer"`
	GuestID  int64  `json:"guest_id,omitempty"`
	SingerID int64  `json:"singer_id,omitempty"`
	Status   string `json:"status"`
	KeyShift int    `json:"key_shift"`
	Duration int64  `json:"duration_ms"`
	Source   string `json:"source"`
	Note     string `json:"note,omitempty"`
}

func toLive(e store.Entry, guest bool) liveEntry {
	le := liveEntry{ID: e.ID, SongID: e.SongID, Title: e.Title, Artist: e.Artist, Singer: e.Singer, Status: e.Status, KeyShift: e.KeyShift, Duration: e.Duration, Source: e.Source, Note: e.Note}
	if e.GuestID != nil {
		le.GuestID = *e.GuestID
	}
	if e.SingerID != nil && !guest {
		le.SingerID = *e.SingerID
	}
	if guest {
		le.Singer = firstName(le.Singer)
		le.Note = ""
	}
	return le
}

// firstName keeps "Ana & Marko" and "Ana" but drops surnames for guests' phones.
func firstName(v string) string {
	parts := strings.Split(v, "&")
	for i, p := range parts {
		f := strings.Fields(p)
		if len(f) > 0 {
			parts[i] = f[0]
		}
	}
	return strings.Join(parts, " & ")
}

// lastResult is the score of the song that just ended.
type lastResult struct {
	EntryID int64    `json:"entry_id"`
	Singer  string   `json:"singer"`
	Title   string   `json:"title"`
	Score   *int     `json:"score"`
	OnTime  *int     `json:"on_time"`
	Steady  *int     `json:"steady"`
	Cheers  int      `json:"cheers"`
	Stars   *float64 `json:"stars"`
	Votes   int      `json:"votes"`
	Best    bool     `json:"best"` // the best score of the night so far
}

type liveState struct {
	Type       string      `json:"type"`
	NightID    int64       `json:"night_id"`
	Title      string      `json:"title"`
	Status     string      `json:"status"`
	Phase      string      `json:"phase"`
	PhaseAt    string      `json:"phase_at"`
	Now        string      `json:"now"`
	Autoplay   int         `json:"autoplay"`
	PositionMs int64       `json:"position_ms"`
	KeyNow     int         `json:"key_now"`
	VoiceNow   int         `json:"voice_now"`
	Volume     int         `json:"volume"`
	Scores     bool        `json:"scores"`
	JoinCode   string      `json:"join_code"`
	JoinURL    string      `json:"join_url"`
	Current    *liveEntry  `json:"current"`
	Song       *liveSong   `json:"song,omitempty"`
	Next       *liveSong   `json:"next_song,omitempty"` // the screen loads it early
	Queue      []liveEntry `json:"queue"`
	Done       []liveEntry `json:"done,omitempty"`
	Asked      []liveEntry `json:"asked,omitempty"`
	Result     *lastResult `json:"result,omitempty"`
	Screens    int         `json:"screens"`
	Phones     int         `json:"phones"`
	Guests     int         `json:"guests"`
	Cheers     int         `json:"cheers"`
	Rules      guestRules  `json:"rules"`
	Settings   *screenSet  `json:"settings,omitempty"`
}

// guestRules are what a guest needs to know to ask for a song.
type guestRules struct {
	Approve  bool `json:"approve"`
	YouTube  bool `json:"youtube"`
	PerGuest int  `json:"per_guest"`
	Book     bool `json:"book"`
}

// screenSet are the host's choices the big screen follows.
type screenSet struct {
	CountIn     bool   `json:"count_in"`
	LyricsSize  string `json:"lyrics_size"`
	LyricsStyle string `json:"lyrics_style"`
	NextLine    bool   `json:"next_line"`
	TranslateTo string `json:"translate_to"`
	PitchLine   bool   `json:"pitch_line"`
	EchoAmount  int    `json:"echo_amount"`
	Photos      bool   `json:"photos"`
}

// joinURL is the address guests open (relative when BASE_URL is not set).
func (s *Server) joinURL(code string) string { return s.link("/j/" + code) }

// states builds the three views of a night.
func (s *Server) states(ctx context.Context, n *store.Night) (host, screen, guest liveState) {
	q, _ := s.store.Queue(ctx, n.ID)
	screens, phones := s.live.counts(n.ID)
	phaseAt := ""
	if n.PhaseAt != nil {
		phaseAt = *n.PhaseAt
	}
	base := liveState{Type: "state", NightID: n.ID, Title: n.Title, Status: n.Status, Phase: n.Phase, PhaseAt: phaseAt, Now: store.Now(), Autoplay: n.Autoplay,
		PositionMs: n.PositionMs, KeyNow: n.KeyNow, VoiceNow: n.VoiceNow, Volume: n.Volume, Scores: n.Scores, JoinCode: n.JoinCode, JoinURL: s.joinURL(n.JoinCode),
		Screens: screens, Phones: phones, Guests: n.Guests, Queue: []liveEntry{},
		Rules: guestRules{Approve: n.Approve, YouTube: n.GuestYouTube, PerGuest: n.PerGuest, Book: n.BookID != nil}}
	host, screen, guest = base, base, base
	host.Done, host.Asked = []liveEntry{}, []liveEntry{}
	for _, e := range q {
		switch e.Status {
		case "singing":
			h, g := toLive(e, false), toLive(e, true)
			host.Current, screen.Current, guest.Current = &h, &h, &g
			host.Cheers = s.live.cheersOf(e.ID)
			screen.Cheers, guest.Cheers = host.Cheers, host.Cheers
		case "waiting":
			host.Queue = append(host.Queue, toLive(e, false))
			screen.Queue = append(screen.Queue, toLive(e, true))
			guest.Queue = append(guest.Queue, toLive(e, true))
		case "asked":
			host.Asked = append(host.Asked, toLive(e, false))
		default:
			if len(host.Done) < 100 {
				host.Done = append(host.Done, toLive(e, false))
			}
		}
	}
	// a finished one stays "current" on the score screen
	if host.Current == nil && n.CurrentID != nil && n.Phase == "score" {
		for _, e := range q {
			if e.ID == *n.CurrentID {
				h, g := toLive(e, false), toLive(e, true)
				host.Current, screen.Current, guest.Current = &h, &h, &g
			}
		}
	}
	if n.Phase == "score" && n.CurrentID != nil {
		if p, err := s.store.PerformanceByQueue(ctx, n.UserID, *n.CurrentID); err == nil {
			res := &lastResult{EntryID: *n.CurrentID, Singer: p.Singer, Title: p.Title, Score: p.Score, OnTime: p.OnTime, Steady: p.Steady, Cheers: p.Cheers, Stars: p.Stars, Votes: p.Votes}
			if p.Score != nil {
				res.Best = true
				if list, err := s.store.ListPerformances(ctx, n.UserID, store.PerfFilter{NightID: n.ID, Limit: 500}); err == nil {
					for _, o := range list {
						if o.ID != p.ID && o.Score != nil && *o.Score > *p.Score {
							res.Best = false
						}
					}
				}
			}
			host.Result, screen.Result = res, res
			g := *res
			g.Singer = firstName(g.Singer)
			guest.Result = &g
		}
	}
	screenBase := fmt.Sprintf("/api/v1/public/screen/%s", n.ScreenKey)
	if host.Current != nil {
		if so, err := s.store.GetSong(ctx, n.UserID, host.Current.SongID); err == nil {
			screen.Song = screenSong(so, screenBase)
			host.Song = screenSong(so, "/api/v1")
			host.Song.Lyrics = "" // the host page shows the words only when asked
		}
	}
	if len(screen.Queue) > 0 {
		if so, err := s.store.GetSong(ctx, n.UserID, screen.Queue[0].SongID); err == nil {
			screen.Next = screenSong(so, screenBase)
			screen.Next.Lyrics = ""
		}
	}
	st := s.settingsOf(ctx, n.UserID)
	screen.Settings = &screenSet{CountIn: st.CountIn, LyricsSize: st.LyricsSize, LyricsStyle: st.LyricsStyle, NextLine: st.NextLine, TranslateTo: st.TranslateTo,
		PitchLine: st.PitchLine, EchoAmount: st.EchoAmount, Photos: st.PhotosAlbum != ""}
	guest.JoinURL = s.joinURL(n.JoinCode)
	return
}

// broadcast sends every screen of a night the new state.
func (h *liveHub) broadcast(ctx context.Context, s *Server, nightID int64) {
	n, err := s.store.NightByID(ctx, nightID)
	if err != nil {
		b := []byte(`{"type":"gone"}`)
		h.send(nightID, b, b, b)
		return
	}
	host, screen, guest := s.states(ctx, n)
	hb, _ := json.Marshal(host)
	sb, _ := json.Marshal(screen)
	gb, _ := json.Marshal(guest)
	h.send(nightID, hb, sb, gb)
}

// stream keeps an event stream open for one screen of a night.
func (s *Server) stream(w http.ResponseWriter, r *http.Request, n *store.Night, role liveRole, guest int64) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming is not supported here")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	sb := s.live.sub(n.ID, role, guest)
	defer func() {
		s.live.unsub(n.ID, sb)
		go s.live.broadcast(context.Background(), s, n.ID) // the counts changed
	}()
	host, screen, gst := s.states(r.Context(), n)
	first := host
	switch role {
	case roleScreen:
		first = screen
	case roleGuest:
		first = gst
	}
	b, _ := json.Marshal(first)
	fmt.Fprintf(w, "data: %s\n\n", b)
	fl.Flush()
	go s.live.broadcast(context.WithoutCancel(r.Context()), s, n.ID)
	beat := time.NewTicker(25 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-sb.ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-beat.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// command tells the big screen to do something now (play, pause, seek…).
func (h *liveHub) command(night int64, cmd string, value int64) {
	b, _ := json.Marshal(map[string]any{"type": "cmd", "cmd": cmd, "value": value, "at": store.Now()})
	h.send(night, nil, b, nil)
}

// reaction shows a cheer on the big screen and the host's page.
func (h *liveHub) reaction(night int64, emoji, name string, total int) {
	b, _ := json.Marshal(map[string]any{"type": "react", "emoji": emoji, "name": firstName(name), "cheers": total})
	h.send(night, b, b, b)
}

// progress passes the screen's position on to the host's page.
func (h *liveHub) progress(night int64, pos int64, playing bool) {
	h.mu.Lock()
	h.pos[night] = pos
	h.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"type": "progress", "position_ms": pos, "playing": playing})
	h.send(night, b, nil, nil)
}

func (s *Server) allowGuest(r *http.Request, what string, n int, per time.Duration) bool {
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	k := what + "|" + ip
	now := time.Now()
	s.guestMu.Lock()
	defer s.guestMu.Unlock()
	keep := []time.Time{}
	for _, t := range s.guestHits[k] {
		if now.Sub(t) < per {
			keep = append(keep, t)
		}
	}
	if len(keep) >= n {
		s.guestHits[k] = keep
		return false
	}
	s.guestHits[k] = append(keep, now)
	return true
}
