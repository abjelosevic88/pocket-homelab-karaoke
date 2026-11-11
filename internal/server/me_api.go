package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

func ctxDetached(ctx context.Context) context.Context { return context.WithoutCancel(ctx) }

// handleHome is the start page: a live night, the next one, songs to sing,
// the latest performances and a few numbers.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	nights, _ := s.store.ListNights(ctx, uid)
	var live, next *store.Night
	for i := range nights {
		n := &nights[i]
		if n.Status == "live" && live == nil {
			live = n
		}
		if n.Status == "planned" && next == nil {
			next = n
		}
	}
	favs, _, _ := s.store.ListSongs(ctx, uid, store.SongFilter{Favourite: true, Sort: "sung", Limit: 12})
	recent, _, _ := s.store.ListSongs(ctx, uid, store.SongFilter{Sort: "added", Limit: 8})
	noWords, nw, _ := s.store.ListSongs(ctx, uid, store.SongFilter{NoWords: true, Limit: 5})
	_, total, _ := s.store.ListSongs(ctx, uid, store.SongFilter{Limit: 1})
	perfs, _ := s.store.ListPerformances(ctx, uid, store.PerfFilter{Limit: 8})
	singers, _ := s.store.ListSingers(ctx, uid)
	copying, _ := s.store.SongsWithCopyState(ctx, "waiting", "working")
	mine := []store.Song{}
	for _, so := range copying {
		if so.UserID == uid {
			mine = append(mine, so)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"live": live, "next": next, "nights": len(nights), "favourites": favs, "recent": recent, "no_words": noWords, "no_words_total": nw,
		"songs": total, "performances": perfs, "singers": len(singers), "copying": mine})
}

// handleSummary is a short overview for other apps (Pocket Hub, AI, Day).
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	_, total, _ := s.store.ListSongs(ctx, uid, store.SongFilter{Limit: 1})
	nights, _ := s.store.ListNights(ctx, uid)
	week := time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339Nano)
	perfs, _ := s.store.ListPerformances(ctx, uid, store.PerfFilter{From: week, Limit: 500})
	var next, live map[string]any
	for _, n := range nights {
		if n.Status == "live" && live == nil {
			live = map[string]any{"id": n.ID, "title": n.Title, "join_code": n.JoinCode, "waiting": n.Waiting, "url": s.link(fmt.Sprintf("/nights/%d", n.ID))}
		}
		if n.Status == "planned" && next == nil {
			next = map[string]any{"id": n.ID, "title": n.Title, "starts_at": n.StartsAt, "place": n.Place, "url": s.link(fmt.Sprintf("/nights/%d", n.ID))}
		}
	}
	top, _ := s.store.TopSongs(ctx, uid, week, 3)
	writeJSON(w, http.StatusOK, map[string]any{"songs": total, "nights": len(nights), "sung_this_week": len(perfs), "live_night": live, "next_night": next, "top_this_week": top, "url": s.link("/")})
}

// handleDay is what happened on one day, for Pocket Day's journal.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	day := chiParam(r, "date")
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "a day is YYYY-MM-DD")
		return
	}
	from, to := t.UTC().Format(time.RFC3339Nano), t.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	perfs, _ := s.store.ListPerformances(ctx, uid, store.PerfFilter{From: from, To: to, Limit: 500})
	items := []map[string]any{}
	nights, _ := s.store.ListNights(ctx, uid)
	for _, n := range nights {
		if strings.HasPrefix(n.StartsAt, day) || (n.StartedAt != nil && localDay(*n.StartedAt) == day) {
			txt := n.Title
			if n.Place != "" {
				txt += " at " + n.Place
			}
			if n.Sung > 0 {
				txt += fmt.Sprintf(" — %d %s sung", n.Sung, plural(n.Sung, "song", "songs"))
			}
			items = append(items, map[string]any{"kind": "night", "id": n.ID, "title": n.Title, "text": txt, "status": n.Status, "time": strings.TrimPrefix(n.StartsAt, day+"T"), "url": s.link(fmt.Sprintf("/nights/%d", n.ID))})
		}
	}
	for _, p := range perfs {
		txt := fmt.Sprintf("%s sang %s", firstNonEmpty(p.Singer, "Someone"), p.Title)
		if p.Artist != "" {
			txt += " (" + p.Artist + ")"
		}
		if p.Score != nil {
			txt += fmt.Sprintf(" — %d points", *p.Score)
		}
		item := map[string]any{"kind": "song", "id": p.ID, "title": p.Title, "artist": p.Artist, "singer": p.Singer, "text": txt, "practice": p.Practice, "time": localClock(p.SungAt)}
		if p.Score != nil {
			item["score"] = *p.Score
		}
		if p.SongID != nil {
			item["url"] = s.link(fmt.Sprintf("/songs/%d", *p.SongID))
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": day, "items": items, "sung": len(perfs)})
}

func localDay(iso string) string {
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		return ""
	}
	return t.In(time.Local).Format("2006-01-02")
}

func localClock(iso string) string {
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		return ""
	}
	return t.In(time.Local).Format("15:04")
}

// handleSearch finds songs (title, artist, tags, words), singers and nights.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	hits := []map[string]any{}
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
		return
	}
	lim := queryInt(r, "limit")
	if lim <= 0 || lim > 50 {
		lim = 20
	}
	songs, _, _ := s.store.ListSongs(ctx, uid, store.SongFilter{Q: q, Limit: lim})
	seen := map[int64]bool{}
	for _, so := range songs {
		seen[so.ID] = true
		hits = append(hits, map[string]any{"kind": "song", "id": so.ID, "title": so.Title, "subtitle": so.Artist, "snippet": "", "url": s.link(fmt.Sprintf("/songs/%d", so.ID)), "path": fmt.Sprintf("/songs/%d", so.ID)})
	}
	// the words: a line that has the search in it
	if len(hits) < lim && len([]rune(q)) >= 3 {
		all, _, _ := s.store.ListSongs(ctx, uid, store.SongFilter{Limit: 1000})
		low := strings.ToLower(q)
		for _, so := range all {
			if seen[so.ID] || len(hits) >= lim {
				continue
			}
			if line := matchLine(lyricsPlain(so.Lyrics), low); line != "" {
				hits = append(hits, map[string]any{"kind": "song", "id": so.ID, "title": so.Title, "subtitle": so.Artist, "snippet": "“" + line + "”", "url": s.link(fmt.Sprintf("/songs/%d", so.ID)), "path": fmt.Sprintf("/songs/%d", so.ID)})
			}
		}
	}
	singers, _ := s.store.ListSingers(ctx, uid)
	for _, g := range singers {
		if strings.Contains(strings.ToLower(g.Name), strings.ToLower(q)) {
			hits = append(hits, map[string]any{"kind": "singer", "id": g.ID, "title": g.Name, "subtitle": fmt.Sprintf("%d %s sung", g.Sung, plural(g.Sung, "song", "songs")), "url": s.link(fmt.Sprintf("/singers/%d", g.ID)), "path": fmt.Sprintf("/singers/%d", g.ID)})
		}
	}
	nights, _ := s.store.ListNights(ctx, uid)
	for _, n := range nights {
		if strings.Contains(strings.ToLower(n.Title+" "+n.Place), strings.ToLower(q)) {
			hits = append(hits, map[string]any{"kind": "night", "id": n.ID, "title": n.Title, "subtitle": strings.TrimSpace(strings.ReplaceAll(n.StartsAt, "T", " ") + " " + n.Place), "url": s.link(fmt.Sprintf("/nights/%d", n.ID)), "path": fmt.Sprintf("/nights/%d", n.ID)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
}

func matchLine(text, low string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(l), low) {
			return clip(strings.TrimSpace(l), 160)
		}
	}
	return ""
}
