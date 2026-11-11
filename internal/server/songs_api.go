package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// ---- songs ----

func (s *Server) songOf(w http.ResponseWriter, r *http.Request) (*store.Song, bool) {
	ctx := r.Context()
	so, err := s.store.GetSong(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return so, true
}

func (s *Server) handleSongs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	f := store.SongFilter{Q: q.Get("q"), Source: q.Get("source"), Language: q.Get("language"), Favourite: queryBool(r, "favourite"), NoWords: queryBool(r, "no_words"),
		Sort: q.Get("sort"), Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")}
	f.BookID = int64(queryInt(r, "book"))
	list, total, err := s.store.ListSongs(ctx, actorID(ctx), f)
	if err != nil {
		s.fail(w, err, "songs")
		return
	}
	langs, _ := s.store.Languages(ctx, actorID(ctx))
	writeJSON(w, http.StatusOK, map[string]any{"songs": list, "total": total, "languages": langs})
}

func (s *Server) handleGetSong(w http.ResponseWriter, r *http.Request) {
	so, ok := s.songOf(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	perfs, _ := s.store.ListPerformances(ctx, actorID(ctx), store.PerfFilter{SongID: so.ID, Limit: 50})
	writeJSON(w, http.StatusOK, map[string]any{"song": so, "performances": perfs, "copy_possible": s.canKeepCopies()})
}

// songIn is what can be changed on a song.
type songIn struct {
	Title          string `json:"title"`
	Artist         string `json:"artist"`
	Language       string `json:"language"`
	Kind           string `json:"kind"`
	Lyrics         string `json:"lyrics"`
	LyricsSource   string `json:"lyrics_source"`
	LyricsOffsetMs int64  `json:"lyrics_offset_ms"`
	VideoWords     bool   `json:"video_words"`
	KeyShift       int    `json:"key_shift"`
	VoiceCut       int    `json:"voice_cut"`
	Tags           string `json:"tags"`
	Notes          string `json:"notes"`
	Favourite      bool   `json:"favourite"`
	DurationMs     int64  `json:"duration_ms"`
	// adding from a link
	URL string `json:"url"`
}

func (s *Server) applySong(so *store.Song, in songIn, keys map[string]bool) error {
	if keys["title"] {
		t := cleanText(in.Title, maxName)
		if t == "" {
			return &userErr{"a song needs a title"}
		}
		so.Title = t
	}
	if keys["artist"] {
		so.Artist = cleanText(in.Artist, maxName)
	}
	if keys["language"] {
		so.Language = cleanLang(in.Language)
	}
	if keys["kind"] {
		if in.Kind != "karaoke" && in.Kind != "original" {
			return &userErr{"a song is a karaoke track or the original"}
		}
		so.Kind = in.Kind
	}
	if keys["lyrics"] {
		so.Lyrics = cleanLyrics(in.Lyrics)
		so.LyricsSynced = isSynced(so.Lyrics)
		so.LyricsSource = "typed"
		if oneOf(in.LyricsSource, []string{"typed", "file", "lrclib", "music", "tapped", "ai", "translate"}) {
			so.LyricsSource = in.LyricsSource
		}
		if so.Lyrics == "" {
			so.LyricsSource = ""
		}
	}
	if keys["lyrics_offset_ms"] {
		so.LyricsOffsetMs = max(-30000, min(30000, in.LyricsOffsetMs))
	}
	if keys["video_words"] {
		so.VideoWords = in.VideoWords
	}
	if keys["key_shift"] {
		so.KeyShift = clampInt(in.KeyShift, -6, 6)
	}
	if keys["voice_cut"] {
		so.VoiceCut = clampInt(in.VoiceCut, 0, 100)
	}
	if keys["tags"] {
		so.Tags = cleanText(strings.ToLower(in.Tags), maxLine)
	}
	if keys["notes"] {
		so.Notes = cleanText(in.Notes, maxNotes)
	}
	if keys["favourite"] {
		so.Favourite = in.Favourite
	}
	if keys["duration_ms"] && in.DurationMs > 0 && so.DurationMs == 0 {
		so.DurationMs = in.DurationMs
	}
	return nil
}

// handleCreateSong adds a song from a link (YouTube, or a song file on the
// web) or a bare title to fill in later (the words first, the song after).
func (s *Server) handleCreateSong(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in songIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if link := strings.TrimSpace(in.URL); link != "" {
		if id := youTubeID(link); id != "" {
			so, created, err := s.songFromYouTube(ctx, u.ID, ytVideo{ID: id}, "")
			if err != nil {
				s.userFail(w, err, "youtube")
				return
			}
			if created {
				delete(keys, "url")
				if len(keys) > 0 {
					_ = s.applySong(so, in, keys)
					_ = s.store.UpdateSong(ctx, so)
				}
				s.afterSongAdded(ctx, u.ID, so)
			}
			writeJSON(w, map[bool]int{true: http.StatusCreated, false: http.StatusOK}[created], so)
			return
		}
		so, err := s.songFromLink(ctx, u.ID, link)
		if err != nil {
			s.userFail(w, err, "link")
			return
		}
		s.afterSongAdded(ctx, u.ID, so)
		writeJSON(w, http.StatusCreated, so)
		return
	}
	so := &store.Song{UserID: u.ID, Kind: "karaoke", Source: "upload", SourceApp: sourceOf(ctx)}
	keys["title"] = true
	if err := s.applySong(so, in, keys); err != nil {
		s.userFail(w, err, "song")
		return
	}
	if err := s.store.CreateSong(ctx, so); err != nil {
		s.fail(w, err, "song")
		return
	}
	s.afterSongAdded(ctx, u.ID, so)
	writeJSON(w, http.StatusCreated, so)
}

func (s *Server) handleUpdateSong(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	so, ok := s.songOf(w, r)
	if !ok {
		return
	}
	if !mayChange(ctx, so.SourceApp) {
		writeErr(w, http.StatusForbidden, "this app may change only the songs it added")
		return
	}
	var in songIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.applySong(so, in, keys); err != nil {
		s.userFail(w, err, "song")
		return
	}
	if keys["lyrics"] && so.Language == "" && so.Lyrics != "" {
		so.Language = guessLanguage(lyricsPlain(so.Lyrics))
	}
	if err := s.store.UpdateSong(ctx, so); err != nil {
		s.fail(w, err, "song")
		return
	}
	s.live.songChanged(ctx, s, so.UserID, so.ID)
	writeJSON(w, http.StatusOK, so)
}

func (s *Server) handleDeleteSong(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	so, ok := s.songOf(w, r)
	if !ok {
		return
	}
	if !mayChange(ctx, so.SourceApp) {
		writeErr(w, http.StatusForbidden, "this app may remove only the songs it added")
		return
	}
	if err := s.store.DeleteSong(ctx, so.UserID, so.ID); err != nil {
		s.fail(w, err, "song")
		return
	}
	s.dropMedia(ctx, so.Media, so.CDG, so.Cover)
	w.WriteHeader(http.StatusNoContent)
}

// handleSongCopy keeps (or drops) a copy of a YouTube song on this server.
func (s *Server) handleSongCopy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	so, ok := s.songOf(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodDelete {
		if so.Source != "youtube" || so.Media == "" {
			writeErr(w, http.StatusBadRequest, "there is no copy to drop")
			return
		}
		old := so.Media
		so.Media, so.MediaSize, so.MediaMime, so.CopyState, so.CopyProgress = "", 0, "", "", 0
		if err := s.store.UpdateSong(ctx, so); err != nil {
			s.fail(w, err, "copy")
			return
		}
		s.dropMedia(ctx, old)
		writeJSON(w, http.StatusOK, so)
		return
	}
	if so.Source != "youtube" {
		writeErr(w, http.StatusBadRequest, "only YouTube songs need a copy")
		return
	}
	if !s.canKeepCopies() {
		writeErr(w, http.StatusBadRequest, "this box cannot keep copies of YouTube songs (yt-dlp is not there, or DOWNLOADS=off)")
		return
	}
	if so.Media != "" {
		writeJSON(w, http.StatusOK, so)
		return
	}
	so.CopyState, so.CopyError, so.CopyProgress = "waiting", "", 0
	if err := s.store.UpdateSong(ctx, so); err != nil {
		s.fail(w, err, "copy")
		return
	}
	s.kickCopier()
	writeJSON(w, http.StatusAccepted, so)
}

// afterSongAdded looks the words up (in the background) and tells the bus.
func (s *Server) afterSongAdded(ctx context.Context, uid int64, so *store.Song) {
	st := s.settingsOf(ctx, uid)
	if st.LyricsLookup && !s.cfg.NoLookups && so.Lyrics == "" && !so.VideoWords && so.CDG == "" {
		cp := *so
		go s.findLyrics(context.WithoutCancel(ctx), uid, &cp)
	}
	if so.CopyState == "waiting" {
		s.kickCopier()
	}
	s.publish(ctx, uid, "karaoke.song_added", fmt.Sprintf("song:%d", so.ID), map[string]any{"song_id": so.ID, "title": so.Title, "artist": so.Artist, "source": so.Source, "url": s.link(fmt.Sprintf("/songs/%d", so.ID))})
}

// songFromLink fetches a song file from the web (a direct link to an MP3 or MP4).
func (s *Server) songFromLink(ctx context.Context, uid int64, link string) (*store.Song, error) {
	clean, ok := cleanURL(link)
	if !ok {
		return nil, &userErr{"that is not a link"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clean, nil)
	if err != nil {
		return nil, &userErr{"that is not a link"}
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, &userErr{"the link did not answer: " + err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, &userErr{fmt.Sprintf("the link answered HTTP %d", res.StatusCode)}
	}
	name := safeName(strings.Split(strings.TrimRight(clean, "/"), "/")[len(strings.Split(strings.TrimRight(clean, "/"), "/"))-1])
	if q := strings.IndexByte(name, '?'); q > 0 {
		name = name[:q]
	}
	ct := strings.Split(res.Header.Get("Content-Type"), ";")[0]
	if mimeByExt(name) == "" {
		for ext, m := range mediaExts {
			if m == ct && (isAudioExt(ext) || isVideoExt(ext)) {
				name += ext
				break
			}
		}
	}
	if !isAudioExt(mimeExt(name)) && !isVideoExt(mimeExt(name)) {
		return nil, &userErr{"the link is not a song file (YouTube links and MP3, M4A, OGG, MP4 or WebM files work)"}
	}
	body := res.Body
	return s.songFromFiles(ctx, uid, []incomingFile{{name: name, open: func() (io.ReadCloser, error) { return body, nil }}}, "link", clean)
}

func mimeExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}

// handleLyricsLookup lists sets of words to choose from (LRCLIB, Pocket Music).
func (s *Server) handleLyricsLookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	so, ok := s.songOf(w, r)
	if !ok {
		return
	}
	title, artist := firstNonEmpty(r.URL.Query().Get("title"), so.Title), r.URL.Query().Get("artist")
	if !r.URL.Query().Has("artist") {
		artist = so.Artist
	}
	out := []lyricsCandidate{}
	if so.Source == "music" {
		if c, err := s.musicLyrics(ctx, so.UserID, so.SourceRef); err == nil && c != nil {
			out = append(out, *c)
		}
	}
	list, err := s.lrclibSearch(ctx, title, artist)
	if err != nil && len(out) == 0 {
		s.userFail(w, err, "lyrics")
		return
	}
	out = append(out, list...)
	writeJSON(w, http.StatusOK, map[string]any{"candidates": out})
}

// ---- singers ----

func (s *Server) handleSingers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListSingers(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "singers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"singers": list})
}

func (s *Server) handleGetSinger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := s.store.GetSinger(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	perfs, _ := s.store.ListPerformances(ctx, actorID(ctx), store.PerfFilter{SingerID: g.ID, Limit: 300})
	writeJSON(w, http.StatusOK, map[string]any{"singer": g, "performances": perfs})
}

var singerColours = []string{"#7b2cbf", "#d6336c", "#1971c2", "#2f9e44", "#e8590c", "#0c8599", "#c2255c", "#5f3dc4", "#e67700", "#087f5b"}

func (s *Server) handleSaveSinger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	g := &store.Singer{UserID: uid}
	if id := idParam(r, "id"); id > 0 {
		cur, err := s.store.GetSinger(ctx, uid, id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		g = cur
	}
	var in store.Singer
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["name"] || g.ID == 0 {
		g.Name = cleanText(in.Name, 60)
		if g.Name == "" {
			writeErr(w, http.StatusBadRequest, "a singer needs a name")
			return
		}
	}
	if keys["colour"] && (in.Colour == "" || reColor.MatchString(in.Colour)) {
		g.Colour = in.Colour
	}
	if g.Colour == "" {
		list, _ := s.store.ListSingers(ctx, uid)
		g.Colour = singerColours[len(list)%len(singerColours)]
	}
	if keys["emoji"] {
		g.Emoji = clip(strings.TrimSpace(in.Emoji), 16)
	}
	if keys["contact_id"] {
		g.ContactID = clip(strings.TrimSpace(in.ContactID), 80)
	}
	if keys["notes"] {
		g.Notes = cleanText(in.Notes, 2000)
	}
	if err := s.store.SaveSinger(ctx, g); err != nil {
		s.fail(w, err, "singer")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteSinger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteSinger(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "singer")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- song books ----

func (s *Server) handleBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListBooks(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "books")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"books": list})
}

func (s *Server) handleSaveBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b := &store.Book{UserID: uid}
	if id := idParam(r, "id"); id > 0 {
		cur, err := s.store.GetBook(ctx, uid, id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		b = cur
	}
	var in struct {
		Name  string  `json:"name"`
		About string  `json:"about"`
		Songs []int64 `json:"songs"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["name"] || b.ID == 0 {
		b.Name = cleanText(in.Name, 80)
		if b.Name == "" {
			writeErr(w, http.StatusBadRequest, "a song book needs a name")
			return
		}
	}
	if keys["about"] {
		b.About = cleanText(in.About, 500)
	}
	if err := s.store.SaveBook(ctx, b); err != nil {
		s.fail(w, err, "book")
		return
	}
	for _, sid := range in.Songs {
		_ = s.store.SetInBook(ctx, uid, b.ID, sid, true)
	}
	b, _ = s.store.GetBook(ctx, uid, b.ID)
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteBook(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "book")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBookSong puts a song in a book (PUT) or takes it out (DELETE).
func (s *Server) handleBookSong(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.SetInBook(ctx, actorID(ctx), idParam(r, "id"), idParam(r, "song"), r.Method == http.MethodPut); err != nil {
		s.fail(w, err, "book")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleYouTubeSearch finds videos to sing (with which of them the person already has).
func (s *Server) handleYouTubeSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"videos": []ytVideo{}})
		return
	}
	if id := youTubeID(q); id != "" {
		v, err := s.youTubeInfo(ctx, id)
		if err != nil {
			s.userFail(w, err, "youtube")
			return
		}
		if old, err := s.store.SongBySource(ctx, actorID(ctx), "youtube", id); err == nil {
			v.Have = old.ID
		}
		writeJSON(w, http.StatusOK, map[string]any{"videos": []ytVideo{v}})
		return
	}
	st := s.settingsOf(ctx, actorID(ctx))
	if suf := strings.TrimSpace(st.YouTubeSuffix); suf != "" && !queryBool(r, "exact") && !strings.Contains(strings.ToLower(q), strings.ToLower(suf)) {
		q += " " + suf
	}
	list, err := s.youTubeSearch(ctx, q)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "YouTube search did not work: "+err.Error())
		return
	}
	out := make([]ytVideo, len(list))
	copy(out, list)
	for i := range out {
		if old, err := s.store.SongBySource(ctx, actorID(ctx), "youtube", out[i].ID); err == nil {
			out[i].Have = old.ID
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"videos": out, "query": q})
}

// handleAddYouTube adds a found video as a song.
func (s *Server) handleAddYouTube(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in ytVideo
	if err := decode(r, &in); err != nil || !reYouTubeID.MatchString(in.ID) {
		writeErr(w, http.StatusBadRequest, "which video?")
		return
	}
	in.Title, in.Channel = cleanText(in.Title, 300), cleanText(in.Channel, 200)
	so, created, err := s.songFromYouTube(ctx, u.ID, in, "")
	if err != nil {
		s.userFail(w, err, "youtube")
		return
	}
	if created {
		s.afterSongAdded(ctx, u.ID, so)
	}
	writeJSON(w, map[bool]int{true: http.StatusCreated, false: http.StatusOK}[created], so)
}
