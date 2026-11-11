package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// ---- the big screen: its own long key, no sign-in (a TV's browser) ----

func (s *Server) screenNight(w http.ResponseWriter, r *http.Request) (*store.Night, bool) {
	n, err := s.store.NightByScreen(r.Context(), chiParam(r, "key"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "this screen link is not in use any more")
		return nil, false
	}
	return n, true
}

func (s *Server) handleScreen(w http.ResponseWriter, r *http.Request) {
	n, ok := s.screenNight(w, r)
	if !ok {
		return
	}
	_, screen, _ := s.states(r.Context(), n)
	writeJSON(w, http.StatusOK, screen)
}

func (s *Server) handleScreenStream(w http.ResponseWriter, r *http.Request) {
	n, ok := s.screenNight(w, r)
	if !ok {
		return
	}
	s.stream(w, r, n, roleScreen, 0)
}

// handleScreenReport: the big screen says what happened (started, progress,
// finished with the score, advance after the score) or a press on it.
func (s *Server) handleScreenReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.screenNight(w, r)
	if !ok {
		return
	}
	var in controlIn
	var extra struct {
		Playing bool `json:"playing"`
	}
	if err := decodeBoth(r, &in, &extra); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	switch in.Action {
	case "progress":
		s.live.progress(n.ID, max(0, in.Value), extra.Playing)
		w.WriteHeader(http.StatusNoContent)
		return
	case "start", "end", "skip", "key", "voice", "volume", "stop":
		writeErr(w, http.StatusForbidden, "the host does that")
		return
	}
	if err := s.control(ctx, n, in, false); err != nil {
		s.userFail(w, err, "screen")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// screenSongOf is a song the screen may play: one in this night's queue.
func (s *Server) screenSongOf(w http.ResponseWriter, r *http.Request) (*store.Song, bool) {
	n, ok := s.screenNight(w, r)
	if !ok {
		return nil, false
	}
	ctx := r.Context()
	id := idParam(r, "id")
	q, _ := s.store.Queue(ctx, n.ID)
	for _, e := range q {
		if e.SongID == id && (e.Status == "singing" || e.Status == "waiting" || e.Status == "done") {
			so, err := s.store.GetSong(ctx, n.UserID, id)
			if err == nil {
				return so, true
			}
		}
	}
	writeErr(w, http.StatusNotFound, "not found")
	return nil, false
}

func (s *Server) handleScreenMedia(w http.ResponseWriter, r *http.Request) {
	so, ok := s.screenSongOf(w, r)
	if !ok {
		return
	}
	s.playSong(w, r, so)
}

func (s *Server) handleScreenCDG(w http.ResponseWriter, r *http.Request) {
	so, ok := s.screenSongOf(w, r)
	if !ok || so.CDG == "" {
		if ok {
			writeErr(w, http.StatusNotFound, "not found")
		}
		return
	}
	s.serveMediaFile(w, r, so.CDG, "application/octet-stream", "")
}

func (s *Server) handleScreenCover(w http.ResponseWriter, r *http.Request) {
	so, ok := s.screenSongOf(w, r)
	if !ok {
		return
	}
	s.serveCover(w, r, so)
}

func (s *Server) handleScreenTranslation(w http.ResponseWriter, r *http.Request) {
	so, ok := s.screenSongOf(w, r)
	if !ok {
		return
	}
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	lines, err := s.translateLyrics(r.Context(), so.UserID, so, to)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"lines": []string{}, "error": plainErr(appTranslate, err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines, "to": to})
}

// ---- guests: the join code, then a name; a cookie keeps them in ----

func guestCookie(nightID int64) string { return fmt.Sprintf("pkar_guest_%d", nightID) }

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func (s *Server) joinNight(w http.ResponseWriter, r *http.Request) (*store.Night, bool) {
	n, err := s.store.NightByCode(r.Context(), chiParam(r, "code"))
	if err != nil || n.Status == "ended" {
		writeErr(w, http.StatusNotFound, "there is no karaoke night with this code (or it is over)")
		return nil, false
	}
	return n, true
}

// guestOf is the guest this phone joined as (nil, nil = not joined yet).
func (s *Server) guestOf(r *http.Request, n *store.Night) *store.Guest {
	c, err := r.Cookie(guestCookie(n.ID))
	if err != nil || len(c.Value) < 20 {
		return nil
	}
	g, err := s.store.GuestByToken(r.Context(), n.ID, hashToken(c.Value))
	if err != nil {
		return nil
	}
	return g
}

func (s *Server) mustGuest(w http.ResponseWriter, r *http.Request) (*store.Night, *store.Guest, bool) {
	n, ok := s.joinNight(w, r)
	if !ok {
		return nil, nil, false
	}
	g := s.guestOf(r, n)
	if g == nil {
		writeErr(w, http.StatusUnauthorized, "say your name first")
		return nil, nil, false
	}
	if g.Blocked {
		writeErr(w, http.StatusForbidden, "the host has paused your requests")
		return nil, nil, false
	}
	return n, g, true
}

func publicNight(n *store.Night) map[string]any {
	return map[string]any{"title": n.Title, "status": n.Status, "place": n.Place, "starts_at": n.StartsAt, "about": n.About,
		"rules": guestRules{Approve: n.Approve, YouTube: n.GuestYouTube, PerGuest: n.PerGuest, Book: n.BookID != nil}}
}

func (s *Server) handleJoinInfo(w http.ResponseWriter, r *http.Request) {
	n, ok := s.joinNight(w, r)
	if !ok {
		return
	}
	out := publicNight(n)
	if g := s.guestOf(r, n); g != nil {
		out["me"] = map[string]any{"id": g.ID, "name": g.Name, "blocked": g.Blocked}
	}
	_, _, guest := s.states(r.Context(), n)
	out["live"] = guest
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.joinNight(w, r)
	if !ok {
		return
	}
	if !s.allowGuest(r, "join", 10, time.Hour) {
		writeErr(w, http.StatusTooManyRequests, "too many tries: wait a little")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := cleanText(in.Name, 40)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "say your name")
		return
	}
	if g := s.guestOf(r, n); g != nil {
		g.Name = name
		_ = s.store.UpdateGuest(ctx, g)
		go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
		writeJSON(w, http.StatusOK, map[string]any{"me": map[string]any{"id": g.ID, "name": g.Name}})
		return
	}
	tok := newToken() + newToken()
	g := &store.Guest{UserID: n.UserID, NightID: n.ID, Name: name}
	if err := s.store.CreateGuest(ctx, g, hashToken(tok)); err != nil {
		s.fail(w, err, "join")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: guestCookie(n.ID), Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies, MaxAge: 2 * 86400})
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"me": map[string]any{"id": g.ID, "name": g.Name}})
}

func (s *Server) handleJoinStream(w http.ResponseWriter, r *http.Request) {
	n, ok := s.joinNight(w, r)
	if !ok {
		return
	}
	var gid int64
	if g := s.guestOf(r, n); g != nil {
		gid = g.ID
	}
	s.stream(w, r, n, roleGuest, gid)
}

// guestSong is a song as a guest's phone lists it.
type guestSong struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Language string `json:"language"`
	Video    bool   `json:"video"`
	Duration int64  `json:"duration_ms"`
	Duet     bool   `json:"duet"`
}

// handleJoinSongs is the song book guests choose from (the night's book, or the host's whole library).
func (s *Server) handleJoinSongs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.joinNight(w, r)
	if !ok {
		return
	}
	f := store.SongFilter{Q: r.URL.Query().Get("q"), Language: r.URL.Query().Get("language"), Sort: firstNonEmpty(r.URL.Query().Get("sort"), "title"), Limit: 60, Offset: queryInt(r, "offset")}
	if n.BookID != nil {
		f.BookID = *n.BookID
	}
	list, total, err := s.store.ListSongs(ctx, n.UserID, f)
	if err != nil {
		s.fail(w, err, "songs")
		return
	}
	out := make([]guestSong, 0, len(list))
	for _, so := range list {
		out = append(out, guestSong{ID: so.ID, Title: so.Title, Artist: so.Artist, Language: so.Language, Video: so.IsVideo, Duration: so.DurationMs, Duet: strings.Contains(so.Tags, "duet")})
	}
	langs, _ := s.store.Languages(ctx, n.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"songs": out, "total": total, "languages": langs})
}

// checkGuestLimit says whether a guest may ask for one more song.
func (s *Server) checkGuestLimit(n *store.Night, g *store.Guest) error {
	if n.PerGuest > 0 && g.Waiting >= n.PerGuest {
		return &userErr{fmt.Sprintf("you have %d %s waiting: ask for more once you have sung", g.Waiting, plural(g.Waiting, "song", "songs"))}
	}
	return nil
}

func (s *Server) guestEntry(ctx context.Context, n *store.Night, g *store.Guest, so *store.Song, with, note string, key int, forceAsk bool) (*store.Entry, error) {
	name := g.Name
	if w := cleanText(with, 40); w != "" {
		name += " & " + w
	}
	e := &store.Entry{UserID: n.UserID, NightID: n.ID, SongID: so.ID, Singer: name, GuestID: &g.ID, SingerID: g.SingerID, KeyShift: clampInt(key, -6, 6), Note: cleanText(note, 200)}
	if e.KeyShift == 0 {
		e.KeyShift = so.KeyShift
	}
	if n.Approve || forceAsk {
		e.Status = "asked"
	}
	if err := s.enqueue(ctx, n, e, false); err != nil {
		return nil, err
	}
	if x, err := s.store.GetEntry(ctx, n.UserID, e.ID); err == nil {
		e = x
	}
	if e.Status == "asked" {
		st := s.settingsOf(ctx, n.UserID)
		if st.NotifyRequest {
			s.push(ctx, n.UserID, st, g.Name+" asks for a song", so.Title+" — "+so.Artist, fmt.Sprintf("/nights/%d", n.ID))
		}
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	return e, nil
}

func (s *Server) handleJoinRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, g, ok := s.mustGuest(w, r)
	if !ok {
		return
	}
	if !s.allowGuest(r, "request", 30, time.Hour) {
		writeErr(w, http.StatusTooManyRequests, "too many requests: wait a little")
		return
	}
	var in struct {
		SongID   int64  `json:"song_id"`
		With     string `json:"with"`
		Note     string `json:"note"`
		KeyShift int    `json:"key_shift"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.checkGuestLimit(n, g); err != nil {
		s.userFail(w, err, "request")
		return
	}
	so, err := s.store.GetSong(ctx, n.UserID, in.SongID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "that song is not in the song book")
		return
	}
	if n.BookID != nil {
		ids, _ := s.store.BookSongIDs(ctx, n.UserID, *n.BookID)
		found := false
		for _, id := range ids {
			found = found || id == so.ID
		}
		if !found {
			writeErr(w, http.StatusNotFound, "that song is not in the song book")
			return
		}
	}
	e, err := s.guestEntry(ctx, n, g, so, in.With, in.Note, in.KeyShift, false)
	if err != nil {
		s.fail(w, err, "request")
		return
	}
	writeJSON(w, http.StatusCreated, toLive(*e, true))
}

func (s *Server) handleJoinCancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, g, ok := s.mustGuest(w, r)
	if !ok {
		return
	}
	e, err := s.store.GetEntry(ctx, n.UserID, idParam(r, "eid"))
	if err != nil || e.NightID != n.ID || e.GuestID == nil || *e.GuestID != g.ID || (e.Status != "waiting" && e.Status != "asked") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.store.DeleteEntry(ctx, n.UserID, e.ID); err != nil {
		s.fail(w, err, "request")
		return
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJoinYouTube(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, _, ok := s.mustGuest(w, r)
	if !ok {
		return
	}
	if !n.GuestYouTube {
		writeErr(w, http.StatusForbidden, "the host chooses the songs from the song book tonight")
		return
	}
	if !s.allowGuest(r, "yt", 40, time.Hour) {
		writeErr(w, http.StatusTooManyRequests, "too many searches: wait a little")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"videos": []ytVideo{}})
		return
	}
	st := s.settingsOf(ctx, n.UserID)
	if suf := strings.TrimSpace(st.YouTubeSuffix); suf != "" && !strings.Contains(strings.ToLower(q), strings.ToLower(suf)) {
		q += " " + suf
	}
	list, err := s.youTubeSearch(ctx, q)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "YouTube search did not work just now")
		return
	}
	out := make([]ytVideo, 0, len(list))
	for _, v := range list {
		v.Have = 0
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"videos": out})
}

// handleJoinYouTubeAsk: a guest asks for a YouTube video; it joins the host's
// library (marked with the guest's name) and the queue.
func (s *Server) handleJoinYouTubeAsk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, g, ok := s.mustGuest(w, r)
	if !ok {
		return
	}
	if !n.GuestYouTube {
		writeErr(w, http.StatusForbidden, "the host chooses the songs from the song book tonight")
		return
	}
	if !s.allowGuest(r, "request", 30, time.Hour) {
		writeErr(w, http.StatusTooManyRequests, "too many requests: wait a little")
		return
	}
	var in struct {
		ytVideo
		With string `json:"with"`
		Note string `json:"note"`
	}
	if err := decode(r, &in); err != nil || !reYouTubeID.MatchString(in.ID) {
		writeErr(w, http.StatusBadRequest, "which video?")
		return
	}
	if err := s.checkGuestLimit(n, g); err != nil {
		s.userFail(w, err, "request")
		return
	}
	v := ytVideo{ID: in.ID, Title: cleanText(in.Title, 300), Channel: cleanText(in.Channel, 200), DurationMs: in.DurationMs}
	v.Karaoke = looksKaraoke(v.Title, v.Channel)
	so, created, err := s.songFromYouTube(ctx, n.UserID, v, g.Name)
	if err != nil {
		s.userFail(w, err, "youtube")
		return
	}
	if created {
		s.afterSongAdded(ctx, n.UserID, so)
	}
	e, err := s.guestEntry(ctx, n, g, so, in.With, in.Note, 0, false)
	if err != nil {
		s.fail(w, err, "request")
		return
	}
	writeJSON(w, http.StatusCreated, toLive(*e, true))
}

var guestEmoji = map[string]bool{"👏": true, "🔥": true, "❤️": true, "😂": true, "🎤": true, "⭐": true, "🙌": true, "😮": true}

func (s *Server) handleJoinReact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, g, ok := s.mustGuest(w, r)
	if !ok {
		return
	}
	var in struct {
		Emoji string `json:"emoji"`
	}
	if err := decode(r, &in); err != nil || !guestEmoji[in.Emoji] {
		writeErr(w, http.StatusBadRequest, "pick one of the cheers")
		return
	}
	key := fmt.Sprintf("%d:%d", n.ID, g.ID)
	s.live.mu.Lock()
	last := s.live.reacts[key]
	if time.Since(last) < 700*time.Millisecond {
		s.live.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.live.reacts[key] = time.Now()
	s.live.mu.Unlock()
	total := 0
	if cur := s.currentEntry(ctx, n); cur != nil {
		total = s.live.addCheer(cur.ID)
	}
	s.live.reaction(n.ID, in.Emoji, g.Name, total)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJoinVote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, g, ok := s.mustGuest(w, r)
	if !ok {
		return
	}
	var in struct {
		EntryID int64 `json:"entry_id"`
		Stars   int   `json:"stars"`
	}
	if err := decode(r, &in); err != nil || in.Stars < 1 || in.Stars > 5 {
		writeErr(w, http.StatusBadRequest, "one to five stars")
		return
	}
	e, err := s.store.GetEntry(ctx, n.UserID, in.EntryID)
	if err != nil || e.NightID != n.ID || (e.Status != "singing" && e.Status != "done") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if e.GuestID != nil && *e.GuestID == g.ID {
		writeErr(w, http.StatusBadRequest, "the room votes for you")
		return
	}
	s.live.vote(e.ID, g.ID, in.Stars)
	if e.Status == "done" {
		s.updateVotes(ctx, n, e.ID)
	}
	go s.live.broadcast(context.WithoutCancel(ctx), s, n.ID)
	writeJSON(w, http.StatusOK, map[string]any{"stars": in.Stars})
}
