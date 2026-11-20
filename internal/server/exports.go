package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// The export is everything a person made, without the song files
// themselves (those are in DATA_DIR/media and in the box's backup): the
// songs with their words and settings, singers, song books, nights and what
// was sung. Importing twice changes nothing.

type exportBook struct {
	Book  store.Book `json:"book"`
	Songs []int64    `json:"songs"` // the songs' ids in this export
}

type exportDoc struct {
	App          string              `json:"app"`
	Version      string              `json:"version"`
	ExportedAt   string              `json:"exported_at"`
	Songs        []store.Song        `json:"songs"`
	Singers      []store.Singer      `json:"singers"`
	Books        []exportBook        `json:"books"`
	Nights       []store.Night       `json:"nights"`
	Performances []store.Performance `json:"performances"`
}

func (s *Server) export(ctx context.Context, u *store.User) (*exportDoc, error) {
	doc := &exportDoc{App: "pocket-karaoke", Version: Version, ExportedAt: store.Now(), Books: []exportBook{}}
	var err error
	if doc.Songs, _, err = s.store.ListSongs(ctx, u.ID, store.SongFilter{Limit: 1000000}); err != nil {
		return nil, err
	}
	if doc.Singers, err = s.store.ListSingers(ctx, u.ID); err != nil {
		return nil, err
	}
	books, _ := s.store.ListBooks(ctx, u.ID)
	for _, b := range books {
		ids, _ := s.store.BookSongIDs(ctx, u.ID, b.ID)
		doc.Books = append(doc.Books, exportBook{Book: b, Songs: ids})
	}
	if doc.Nights, err = s.store.ListNights(ctx, u.ID); err != nil {
		return nil, err
	}
	for i := range doc.Nights {
		doc.Nights[i].JoinCode, doc.Nights[i].ScreenKey = "", "" // a fresh code on import
	}
	if doc.Performances, err = s.store.ListPerformances(ctx, u.ID, store.PerfFilter{Limit: 2000}); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	doc, err := s.export(r.Context(), userFrom(r.Context()))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-karaoke-%s.json"`, time.Now().Format("2006-01-02")))
	_ = json.NewEncoder(w).Encode(doc)
}

// handleExportCSV is one row per song sung.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-karaoke-%s.csv"`, time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"sung_at", "singer", "title", "artist", "night", "practice", "score", "on_time", "steady", "cheers", "stars", "key"})
	list, _ := s.store.ListPerformances(ctx, actorID(ctx), store.PerfFilter{Limit: 2000})
	opt := func(v *int) string {
		if v == nil {
			return ""
		}
		return strconv.Itoa(*v)
	}
	for _, p := range list {
		t, _ := time.Parse(time.RFC3339Nano, p.SungAt)
		stars := ""
		if p.Stars != nil {
			stars = fmt.Sprintf("%.1f", *p.Stars)
		}
		_ = cw.Write([]string{t.Local().Format("2006-01-02 15:04"), p.Singer, p.Title, p.Artist, p.NightTitle, strconv.FormatBool(p.Practice), opt(p.Score), opt(p.OnTime), opt(p.Steady),
			strconv.Itoa(p.Cheers), stars, strconv.Itoa(p.KeyShift)})
	}
	cw.Flush()
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var doc exportDoc
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<20)).Decode(&doc); err != nil || doc.App != "pocket-karaoke" {
		writeErr(w, http.StatusBadRequest, "that is not a Pocket Karaoke export")
		return
	}
	res, err := s.importDoc(ctx, userFrom(ctx), &doc)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func songKey(so store.Song) string {
	if so.Source != "upload" && so.SourceRef != "" {
		return so.Source + "|" + so.SourceRef
	}
	return "t|" + strings.ToLower(strings.TrimSpace(so.Title)) + "|" + strings.ToLower(strings.TrimSpace(so.Artist))
}

// importDoc adds what is not here yet (songs by where they came from, or by
// title and artist; singers and books by name; nights by title and day;
// performances by when, who and what).
func (s *Server) importDoc(ctx context.Context, u *store.User, doc *exportDoc) (map[string]any, error) {
	have, _, err := s.store.ListSongs(ctx, u.ID, store.SongFilter{Limit: 1000000})
	if err != nil {
		return nil, err
	}
	byKey := map[string]int64{}
	for _, so := range have {
		byKey[songKey(so)] = so.ID
	}
	songMap := map[int64]int64{}
	added := map[string]int{}
	for _, so := range doc.Songs {
		old := so.ID
		if id, ok := byKey[songKey(so)]; ok {
			songMap[old] = id
			continue
		}
		so.ID, so.UserID, so.SourceApp = 0, u.ID, sourceOf(ctx)
		so.Media, so.CDG, so.Cover = "", "", "" // the files are not in the export
		so.MediaSize = 0
		if so.Source == "youtube" {
			so.CopyState, so.CopyProgress, so.CopyError = "", 0, ""
		}
		so.Title = firstNonEmpty(cleanText(so.Title, maxName), "Untitled song")
		so.Lyrics = cleanLyrics(so.Lyrics)
		so.LastSungAt = nil
		so.Plays = 0
		if err := s.store.CreateSong(ctx, &so); err != nil {
			return nil, err
		}
		byKey[songKey(so)] = so.ID
		songMap[old] = so.ID
		added["songs"]++
	}
	singers, _ := s.store.ListSingers(ctx, u.ID)
	singerByName := map[string]int64{}
	for _, g := range singers {
		singerByName[strings.ToLower(g.Name)] = g.ID
	}
	singerMap := map[int64]int64{}
	for _, g := range doc.Singers {
		old := g.ID
		if id, ok := singerByName[strings.ToLower(g.Name)]; ok {
			singerMap[old] = id
			continue
		}
		g.ID, g.UserID = 0, u.ID
		if strings.TrimSpace(g.Name) == "" {
			continue
		}
		if err := s.store.SaveSinger(ctx, &g); err != nil {
			return nil, err
		}
		singerByName[strings.ToLower(g.Name)] = g.ID
		singerMap[old] = g.ID
		added["singers"]++
	}
	books, _ := s.store.ListBooks(ctx, u.ID)
	bookByName := map[string]int64{}
	for _, b := range books {
		bookByName[strings.ToLower(b.Name)] = b.ID
	}
	bookMap := map[int64]int64{}
	for _, eb := range doc.Books {
		b := eb.Book
		old := b.ID
		id, ok := bookByName[strings.ToLower(b.Name)]
		if !ok {
			b.ID, b.UserID = 0, u.ID
			if err := s.store.SaveBook(ctx, &b); err != nil {
				return nil, err
			}
			id = b.ID
			bookByName[strings.ToLower(b.Name)] = id
			added["books"]++
		}
		bookMap[old] = id
		for _, sid := range eb.Songs {
			if nid, ok := songMap[sid]; ok {
				_ = s.store.SetInBook(ctx, u.ID, id, nid, true)
			}
		}
	}
	nights, _ := s.store.ListNights(ctx, u.ID)
	nightKey := func(n store.Night) string {
		return strings.ToLower(n.Title) + "|" + n.StartsAt + "|" + firstNonEmpty(derefS(n.StartedAt), "")
	}
	nightBy := map[string]int64{}
	for _, n := range nights {
		nightBy[nightKey(n)] = n.ID
	}
	nightMap := map[int64]int64{}
	for _, n := range doc.Nights {
		old := n.ID
		if id, ok := nightBy[nightKey(n)]; ok {
			nightMap[old] = id
			continue
		}
		n.ID, n.UserID, n.SourceApp = 0, u.ID, sourceOf(ctx)
		n.JoinCode, n.ScreenKey = newJoinCode(), newToken()+newToken()[:8]
		n.CurrentID, n.Phase, n.PhaseAt = nil, "idle", nil
		if n.Status == "live" {
			n.Status = "ended"
		}
		if n.BookID != nil {
			if id, ok := bookMap[*n.BookID]; ok {
				n.BookID = &id
			} else {
				n.BookID = nil
			}
		}
		if err := s.store.CreateNight(ctx, &n); err != nil {
			return nil, err
		}
		nightBy[nightKey(n)] = n.ID
		nightMap[old] = n.ID
		added["nights"]++
	}
	perfs, _ := s.store.ListPerformances(ctx, u.ID, store.PerfFilter{Limit: 2000})
	perfKey := func(p store.Performance) string {
		return p.SungAt + "|" + strings.ToLower(p.Singer) + "|" + strings.ToLower(p.Title)
	}
	seen := map[string]bool{}
	for _, p := range perfs {
		seen[perfKey(p)] = true
	}
	for _, p := range doc.Performances {
		if seen[perfKey(p)] {
			continue
		}
		p.ID, p.UserID, p.QueueID, p.GuestID, p.Recording, p.RecordingMime = 0, u.ID, nil, nil, "", ""
		remap := func(v *int64, m map[int64]int64) *int64 {
			if v == nil {
				return nil
			}
			if id, ok := m[*v]; ok {
				return &id
			}
			return nil
		}
		p.SongID, p.SingerID, p.NightID = remap(p.SongID, songMap), remap(p.SingerID, singerMap), remap(p.NightID, nightMap)
		if err := s.store.CreatePerformance(ctx, &p); err != nil {
			return nil, err
		}
		seen[perfKey(p)] = true
		added["performances"]++
	}
	// the counts also at the top level: what Pocket Move shows
	return map[string]any{"ok": true, "added": added, "songs": added["songs"], "singers": added["singers"], "books": added["books"], "nights": added["nights"], "performances": added["performances"], "text": fmt.Sprintf("Added %d songs, %d singers, %d song books, %d nights and %d performances", added["songs"], added["singers"], added["books"], added["nights"], added["performances"])}, nil
}

func derefS(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
