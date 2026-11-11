package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// The other Pocket apps. None of it is needed: when an app is not there,
// the page greys the button and says what to do.

func linkTo(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

func plainErr(a pocketApp, err error) string {
	if isNotConnected(err) {
		return err.Error()
	}
	if ue, ok := err.(*userErr); ok {
		return ue.msg
	}
	return a.name + ": " + err.Error()
}

func (s *Server) appFail(w http.ResponseWriter, a pocketApp, err error) {
	if isNotConnected(err) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if ue, ok := err.(*userErr); ok {
		writeErr(w, http.StatusBadRequest, ue.msg)
		return
	}
	writeErr(w, http.StatusBadGateway, plainErr(a, err))
}

// handleApps lists how the person reaches each app, in words they can act on.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

func (s *Server) sib(w http.ResponseWriter, r *http.Request, a pocketApp, scopes ...string) (*siblingConn, bool) {
	ctx := r.Context()
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, a, scopes...)
	if err != nil {
		s.appFail(w, a, err)
		return nil, false
	}
	return sib, true
}

func (s *Server) actorName(u *store.User) string {
	if u == nil {
		return "Someone"
	}
	if n := strings.TrimSpace(u.Name); n != "" {
		return n
	}
	if i := strings.Index(u.Email, "@"); i > 0 {
		return u.Email[:i]
	}
	return "Someone"
}

// ---- Pocket Music: sing your own library ----

var reMusicID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// handleMusicSearch finds songs in the person's music library.
func (s *Server) handleMusicSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, ok := s.sib(w, r, appMusic, scopeMusicPlay)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"songs": []any{}})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}, "limit": {"40"}})
	if err != nil {
		s.appFail(w, appMusic, err)
		return
	}
	m, _ := v.(map[string]any)
	out := []map[string]any{}
	for _, so := range rows(m["songs"]) {
		id := str(so, "id")
		if !reMusicID.MatchString(id) {
			continue
		}
		item := map[string]any{"id": id, "title": str(so, "title"), "artist": str(so, "artist"), "album": str(so, "album"), "year": num(so, "year"), "duration_ms": int64(num(so, "duration") * 1000),
			"cover": "/api/v1/apps/music/cover/" + url.PathEscape(firstNonEmpty(str(so, "album_id"), id))}
		if old, err := s.store.SongBySource(ctx, actorID(ctx), "music", id); err == nil {
			item["have"] = old.ID
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"songs": out, "url": linkTo(sib, "/")})
}

func (s *Server) handleMusicCover(w http.ResponseWriter, r *http.Request) {
	id := chiParam(r, "id")
	if !reMusicID.MatchString(id) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	sib, ok := s.sib(w, r, appMusic, scopeMusicPlay)
	if !ok {
		return
	}
	b, ctype, err := sib.client.Bytes(r.Context(), "/api/v1/cover/"+url.PathEscape(id), url.Values{"size": {"300"}}, 8<<20)
	if err != nil {
		s.appFail(w, appMusic, err)
		return
	}
	w.Header().Set("Content-Type", firstNonEmpty(ctype, http.DetectContentType(b)))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(b)
}

// handleMusicAdd adds songs from Pocket Music; they play from there (nothing is copied).
func (s *Server) handleMusicAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &in); err != nil || len(in.IDs) == 0 || len(in.IDs) > 100 {
		writeErr(w, http.StatusBadRequest, "pick one or more songs")
		return
	}
	sib, ok := s.sib(w, r, appMusic, scopeMusicPlay)
	if !ok {
		return
	}
	added := []store.Song{}
	for _, id := range in.IDs {
		if !reMusicID.MatchString(id) {
			continue
		}
		if old, err := s.store.SongBySource(ctx, u.ID, "music", id); err == nil {
			added = append(added, *old)
			continue
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/songs/"+url.PathEscape(id), nil)
		if err != nil {
			s.appFail(w, appMusic, err)
			return
		}
		m, _ := v.(map[string]any)
		if inner, ok := m["song"].(map[string]any); ok {
			m = inner
		}
		so := &store.Song{UserID: u.ID, Title: firstNonEmpty(cleanText(str(m, "title"), maxName), "Untitled song"), Artist: cleanText(str(m, "artist"), maxName), Kind: "original",
			Source: "music", SourceRef: id, DurationMs: int64(num(m, "duration") * 1000), MediaMime: mimeByExt("x." + str(m, "suffix")), SourceApp: sourceOf(ctx),
			VoiceCut: s.settingsOf(ctx, u.ID).DefaultVoice}
		if t := strings.ToLower(str(m, "title") + " " + str(m, "album")); strings.Contains(t, "karaoke") || strings.Contains(t, "instrumental") {
			so.Kind, so.VoiceCut = "karaoke", 0
		}
		if c, err := s.musicLyricsWith(ctx, sib, id); err == nil && c != nil {
			so.Lyrics, so.LyricsSynced, so.LyricsSource = cleanLyrics(c.Lyrics), c.Synced, "music"
			so.Language = guessLanguage(lyricsPlain(so.Lyrics))
		}
		if err := s.store.CreateSong(ctx, so); err != nil {
			s.fail(w, err, "song")
			return
		}
		s.afterSongAdded(ctx, u.ID, so)
		added = append(added, *so)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"songs": added, "text": fmt.Sprintf("%d %s from Pocket Music", len(added), plural(len(added), "song", "songs"))})
}

// streamFromMusic plays a Pocket Music song through the person's link, with ranges.
func (s *Server) streamFromMusic(w http.ResponseWriter, r *http.Request, so *store.Song) {
	ctx := r.Context()
	sib, err := s.clients(ctx, so.UserID).conn(ctx, appMusic, scopeMusicPlay)
	if err != nil {
		s.appFail(w, appMusic, err)
		return
	}
	res, err := sib.client.Open(ctx, "/api/v1/stream/"+url.PathEscape(so.SourceRef), nil, r.Header.Get("Range"))
	if err != nil {
		if ctx.Err() == nil {
			s.appFail(w, appMusic, err)
		}
		return
	}
	defer res.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := res.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

func (s *Server) coverFromMusic(w http.ResponseWriter, r *http.Request, so *store.Song) {
	ctx := r.Context()
	sib, err := s.clients(ctx, so.UserID).conn(ctx, appMusic, scopeMusicPlay)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no picture")
		return
	}
	b, ctype, err := sib.client.Bytes(ctx, "/api/v1/cover/"+url.PathEscape(so.SourceRef), url.Values{"size": {"600"}}, 8<<20)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no picture")
		return
	}
	w.Header().Set("Content-Type", firstNonEmpty(ctype, http.DetectContentType(b)))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(b)
}

func (s *Server) musicLyrics(ctx context.Context, uid int64, id string) (*lyricsCandidate, error) {
	sib, err := s.clients(ctx, uid).conn(ctx, appMusic, scopeMusicPlay)
	if err != nil {
		return nil, err
	}
	return s.musicLyricsWith(ctx, sib, id)
}

func (s *Server) musicLyricsWith(ctx context.Context, sib *siblingConn, id string) (*lyricsCandidate, error) {
	v, err := getJSONFrom(ctx, sib, "/api/v1/lyrics/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	lines := []lyricLine{}
	synced := m["synced"] == true
	for _, l := range rows(m["lines"]) {
		t := int64(-1)
		if x, ok := l["t"].(float64); ok {
			t = int64(x)
		}
		if t < 0 {
			synced = false
		}
		lines = append(lines, lyricLine{T: t, Text: str(l, "text")})
	}
	if len(lines) == 0 {
		return nil, nil
	}
	txt := ""
	if synced {
		txt = toLRC(lines)
	} else {
		var b strings.Builder
		for _, l := range lines {
			b.WriteString(l.Text + "\n")
		}
		txt = strings.TrimSpace(b.String())
	}
	return &lyricsCandidate{Source: "music", Synced: synced, Lyrics: txt, Preview: clip(strings.Join(firstLines(lyricsPlain(txt), 3), " / "), 160), Score: 100}, nil
}

// nightPlaylist makes a Pocket Music playlist of the night's songs (the
// originals the person has in Pocket Music, found by title and artist).
func (s *Server) nightPlaylist(ctx context.Context, uid int64, n *store.Night, perfs []store.Performance) (string, error) {
	sib, err := s.clients(ctx, uid).conn(ctx, appMusic, scopePlaylistsWrite)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	ids := []string{}
	for i := len(perfs) - 1; i >= 0; i-- {
		p := perfs[i]
		key := strings.ToLower(p.Title + "|" + p.Artist)
		if seen[key] {
			continue
		}
		seen[key] = true
		if p.SongID != nil {
			if so, err := s.store.GetSong(ctx, uid, *p.SongID); err == nil && so.Source == "music" {
				ids = append(ids, so.SourceRef)
				continue
			}
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {cleanForLookup(p.Title) + " " + p.Artist}, "limit": {"5"}})
		if err != nil {
			continue
		}
		m, _ := v.(map[string]any)
		for _, so := range rows(m["songs"]) {
			if matchScore(cleanForLookup(p.Title), p.Artist, str(so, "title"), str(so, "artist")) >= 10 {
				ids = append(ids, str(so, "id"))
				break
			}
		}
	}
	if len(ids) == 0 {
		return "", &userErr{"none of the night's songs are in Pocket Music"}
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/playlists", map[string]any{"name": clip("Karaoke: "+n.Title, 100), "comment": "What was sung at " + n.Title + " (Pocket Karaoke)", "song_ids": ids})
	if err != nil {
		return "", err
	}
	return linkTo(sib, "/playlists/"+url.PathEscape(firstNonEmpty(str(res, "id"), idOf(res)))), nil
}

func (s *Server) handleNightPlaylist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	perfs, _ := s.store.ListPerformances(ctx, n.UserID, store.PerfFilter{NightID: n.ID, Limit: 500})
	if len(perfs) == 0 {
		writeErr(w, http.StatusBadRequest, "nothing has been sung at this night yet")
		return
	}
	link, err := s.nightPlaylist(ctx, n.UserID, n, perfs)
	if err != nil {
		s.appFail(w, appMusic, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": link, "text": "The playlist is in Pocket Music"})
}

// ---- Pocket Files ----

// handleFilesList lists a folder: folders and karaoke files.
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, ok := s.sib(w, r, appFiles, scopeFilesRead)
	if !ok {
		return
	}
	space := firstNonEmpty(r.URL.Query().Get("space"), "home")
	path := firstNonEmpty(r.URL.Query().Get("path"), "/")
	v, err := getJSONFrom(ctx, sib, "/api/v1/fs/list", url.Values{"space": {space}, "path": {path}})
	if err != nil {
		s.appFail(w, appFiles, err)
		return
	}
	out := []map[string]any{}
	for _, e := range rows(v, "entries") {
		name := str(e, "name")
		dir := e["is_dir"] == true
		ext := strings.ToLower(filepath.Ext(name))
		kind := ""
		switch {
		case dir:
			kind = "folder"
		case isAudioExt(ext):
			kind = "audio"
		case isVideoExt(ext):
			kind = "video"
		case ext == ".zip":
			kind = "zip"
		case ext == ".cdg":
			kind = "cdg"
		case ext == ".lrc":
			kind = "lrc"
		}
		if kind == "" || strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, map[string]any{"name": name, "path": str(e, "path"), "kind": kind, "size": num(e, "size"), "modified": str(e, "modified")})
	}
	spaces := []map[string]any{}
	if sv, err := getJSONFrom(ctx, sib, "/api/v1/spaces", nil); err == nil {
		for _, sp := range rows(sv, "spaces") {
			spaces = append(spaces, map[string]any{"id": str(sp, "id"), "name": str(sp, "name")})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out, "space": space, "path": path, "spaces": spaces})
}

// handleFromFiles adds songs from Pocket Files: each audio or video file a
// song, with the .cdg and .lrc of the same name beside it.
func (s *Server) handleFromFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		Space string   `json:"space"`
		Paths []string `json:"paths"`
	}
	if err := decode(r, &in); err != nil || len(in.Paths) == 0 || len(in.Paths) > 50 {
		writeErr(w, http.StatusBadRequest, "pick one or more files")
		return
	}
	sib, ok := s.sib(w, r, appFiles, scopeFilesRead)
	if !ok {
		return
	}
	space := firstNonEmpty(in.Space, "home")
	// the folder's other files, for the .cdg/.lrc beside each song
	siblingsOf := map[string][]string{}
	tmpDir := filepath.Join(s.cfg.DataDir, "tmp")
	_ = os.MkdirAll(tmpDir, 0o755)
	fetch := func(path string) (incomingFile, func(), error) {
		res, err := sib.client.Open(ctx, "/api/v1/fs/raw", url.Values{"space": {space}, "path": {path}}, "")
		if err != nil {
			return incomingFile{}, nil, err
		}
		defer res.Body.Close()
		tf, err := os.CreateTemp(tmpDir, "files-*"+strings.ToLower(filepath.Ext(path)))
		if err != nil {
			return incomingFile{}, nil, err
		}
		_, err = io.Copy(tf, io.LimitReader(res.Body, maxSongBytes))
		tf.Close()
		p := tf.Name()
		done := func() { _ = os.Remove(p) }
		if err != nil {
			done()
			return incomingFile{}, nil, err
		}
		return incomingFile{name: filepath.Base(path), open: func() (io.ReadCloser, error) { return os.Open(p) }}, done, nil
	}
	added := []store.Song{}
	var errs []string
	for _, p := range in.Paths {
		ext := strings.ToLower(filepath.Ext(p))
		if !isAudioExt(ext) && !isVideoExt(ext) && ext != ".zip" {
			continue
		}
		dir := filepath.Dir(p)
		if _, ok := siblingsOf[dir]; !ok {
			names := []string{}
			if v, err := getJSONFrom(ctx, sib, "/api/v1/fs/list", url.Values{"space": {space}, "path": {dir}}); err == nil {
				for _, e := range rows(v, "entries") {
					names = append(names, str(e, "path"))
				}
			}
			siblingsOf[dir] = names
		}
		ref := space + ":" + p
		if old, err := s.store.SongBySource(ctx, u.ID, "files", ref); err == nil {
			added = append(added, *old)
			continue
		}
		files := []incomingFile{}
		var cleanups []func()
		f, done, err := fetch(p)
		if err != nil {
			errs = append(errs, filepath.Base(p)+": "+err.Error())
			continue
		}
		files = append(files, f)
		cleanups = append(cleanups, done)
		stem := strings.TrimSuffix(p, filepath.Ext(p))
		for _, other := range siblingsOf[dir] {
			oe := strings.ToLower(filepath.Ext(other))
			if strings.EqualFold(strings.TrimSuffix(other, filepath.Ext(other)), stem) && (oe == ".cdg" || oe == ".lrc") {
				if f2, done2, err := fetch(other); err == nil {
					files = append(files, f2)
					cleanups = append(cleanups, done2)
				}
			}
		}
		so, err := s.songFromFiles(ctx, u.ID, files, "files", ref)
		for _, c := range cleanups {
			c()
		}
		if err != nil {
			errs = append(errs, filepath.Base(p)+": "+plainErr(appFiles, err))
			continue
		}
		s.afterSongAdded(ctx, u.ID, so)
		added = append(added, *so)
	}
	if len(added) == 0 && len(errs) > 0 {
		writeErr(w, http.StatusBadRequest, strings.Join(errs, "; "))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"songs": added, "errors": errs, "text": fmt.Sprintf("%d %s from your files", len(added), plural(len(added), "song", "songs"))})
}

// handleRecordingToFiles keeps a performance's recording in Pocket Files.
func (s *Server) handleRecordingToFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := s.perfOf(w, r)
	if !ok {
		return
	}
	if p.Recording == "" {
		writeErr(w, http.StatusBadRequest, "there is no recording of this one")
		return
	}
	sib, ok := s.sib(w, r, appFiles, scopeFilesWrite)
	if !ok {
		return
	}
	data, err := os.ReadFile(s.mediaPath(p.Recording))
	if err != nil {
		writeErr(w, http.StatusNotFound, "the recording is gone")
		return
	}
	at, _ := time.Parse(time.RFC3339Nano, p.SungAt)
	name := fileSafe(fmt.Sprintf("%s - %s - %s", at.In(time.Local).Format("2006-01-02"), firstNonEmpty(p.Singer, "Someone"), p.Title)) + extForMime(p.RecordingMime)
	dir := strings.Trim(firstNonEmpty(s.settingsOf(ctx, p.UserID).FilesFolder, "Karaoke"), "/")
	var res map[string]any
	q := url.Values{"space": {"home"}, "path": {"/"}, "name": {dir + "/" + name}, "conflict": {"rename"}, "mtime": {strconv.FormatInt(time.Now().UnixMilli(), 10)}}
	if err := sib.client.PutRaw(ctx, "/api/v1/fs/upload", q, data, firstNonEmpty(p.RecordingMime, "audio/webm"), &res); err != nil {
		s.appFail(w, appFiles, err)
		return
	}
	path := firstNonEmpty(str(res, "path"), "/"+dir+"/"+name)
	folder := path
	if i := strings.LastIndex(path, "/"); i > 0 {
		folder = path[:i]
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Saved in Pocket Files as " + path, "url": linkTo(sib, "/f/home"+folder)})
}

// ---- Pocket Photos: pictures between songs ----

func (s *Server) handlePhotoAlbums(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appPhotos, scopePhotosRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/immich/albums", nil)
	if err != nil {
		s.appFail(w, appPhotos, err)
		return
	}
	out := []map[string]any{}
	for _, a := range rows(v, "albums") {
		if id := str(a, "id"); reUUID.MatchString(id) {
			out = append(out, map[string]any{"id": id, "name": str(a, "albumName"), "count": num(a, "assetCount")})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": out})
}

// screenPhotos lists up to 60 pictures of the person's chosen album (cached ten minutes).
func (s *Server) screenPhotos(ctx context.Context, uid int64) ([]string, error) {
	st := s.settingsOf(ctx, uid)
	if !reUUID.MatchString(st.PhotosAlbum) {
		return []string{}, nil
	}
	key := fmt.Sprintf("photos:%d:%s", uid, st.PhotosAlbum)
	if v, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < 10*time.Minute {
			var ids []string
			if json.Unmarshal([]byte(v), &ids) == nil {
				return ids, nil
			}
		}
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appPhotos, scopePhotosRead)
	if err != nil {
		return nil, err
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/immich/search/metadata", map[string]any{"albumIds": []string{st.PhotosAlbum}, "type": "IMAGE", "size": 60, "order": "desc"})
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, m := range rows(obj(res, "assets")["items"]) {
		if id := str(m, "id"); reUUID.MatchString(id) {
			ids = append(ids, id)
		}
	}
	b, _ := json.Marshal(ids)
	_ = s.store.CachePut(ctx, key, string(b))
	return ids, nil
}

func (s *Server) handleScreenPhotos(w http.ResponseWriter, r *http.Request) {
	n, ok := s.screenNight(w, r)
	if !ok {
		return
	}
	ids, err := s.screenPhotos(r.Context(), n.UserID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"photos": []string{}})
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, fmt.Sprintf("/api/v1/public/screen/%s/photos/%s", n.ScreenKey, id))
	}
	writeJSON(w, http.StatusOK, map[string]any{"photos": out})
}

func (s *Server) handleScreenPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.screenNight(w, r)
	if !ok {
		return
	}
	id := chiParam(r, "uuid")
	ids, _ := s.screenPhotos(ctx, n.UserID)
	found := false
	for _, x := range ids {
		found = found || x == id
	}
	if !found {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	sib, err := s.clients(ctx, n.UserID).conn(ctx, appPhotos, scopePhotosRead)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	b, ctype, err := sib.client.Bytes(ctx, "/api/v1/immich/assets/"+id+"/thumbnail", url.Values{"size": {"preview"}}, 20<<20)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", firstNonEmpty(ctype, http.DetectContentType(b)))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(b)
}

// ---- Pocket Calendar ----

// nightToCalendar puts a planned night in the person's calendar with the join link.
func (s *Server) nightToCalendar(ctx context.Context, uid int64, n *store.Night) (string, error) {
	sib, err := s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return "", err
	}
	if n.StartsAt == "" {
		return "", &userErr{"set the day of the night first"}
	}
	desc := "🎤 Guests join on their phones with the code " + n.JoinCode + ": " + s.joinURL(n.JoinCode)
	if n.About != "" {
		desc = n.About + "\n\n" + desc
	}
	body := map[string]any{"title": clip(n.Title, 200), "description": desc, "url": s.link(fmt.Sprintf("/nights/%d", n.ID)), "location": n.Place, "categories": []string{"Karaoke"}}
	if len(n.StartsAt) == 10 {
		body["all_day"], body["start"], body["end"] = true, n.StartsAt, n.StartsAt
	} else {
		start, err := time.ParseInLocation("2006-01-02T15:04", n.StartsAt[:16], time.Local)
		if err != nil {
			return "", &userErr{"the night's time is YYYY-MM-DDTHH:MM"}
		}
		body["start"], body["end"] = start.Format(time.RFC3339), start.Add(3*time.Hour).Format(time.RFC3339)
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/events", body)
	if err != nil {
		return "", err
	}
	return linkTo(sib, "/?event="+url.QueryEscape(firstNonEmpty(str(res, "event_id"), idOf(res)))), nil
}

func (s *Server) handleNightCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	link, err := s.nightToCalendar(ctx, n.UserID, n)
	if err != nil {
		s.appFail(w, appCalendar, err)
		return
	}
	n.CalendarRef = link
	_ = s.store.UpdateNight(ctx, n)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": link, "text": "The night is in Pocket Calendar"})
}

// ---- Pocket Chat ----

func (s *Server) handleChats(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appChat, scopeChatRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/conversations", nil)
	if err != nil {
		s.appFail(w, appChat, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "conversations") {
		if m["archived"] == true {
			continue
		}
		out = append(out, map[string]any{"id": idOf(m), "name": firstNonEmpty(str(m, "name"), str(m, "title")), "kind": str(m, "kind"), "emoji": str(m, "emoji")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

// handleNightChat sends a night's invitation (the code and the link) or its results to a chat.
func (s *Server) handleNightChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, ok := s.nightOf(w, r)
	if !ok {
		return
	}
	var in struct {
		To   string `json:"to"`
		What string `json:"what"` // invite | results
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	sib, ok := s.sib(w, r, appChat, scopeChatWrite)
	if !ok {
		return
	}
	var card map[string]any
	text := ""
	if in.What == "results" {
		perfs, _ := s.store.ListPerformances(ctx, n.UserID, store.PerfFilter{NightID: n.ID, Limit: 500})
		if len(perfs) == 0 {
			writeErr(w, http.StatusBadRequest, "nothing has been sung at this night yet")
			return
		}
		lines := nightResultLines(perfs)
		text = fmt.Sprintf("🎤 %s: %d %s sung.", n.Title, len(perfs), plural(len(perfs), "song", "songs"))
		card = map[string]any{"title": n.Title, "subtitle": "How the night went", "lines": lines, "icon": "🏆", "url": s.link(fmt.Sprintf("/nights/%d", n.ID)), "app": "karaoke", "kind": "card"}
	} else {
		when := strings.ReplaceAll(n.StartsAt, "T", " at ")
		text = "🎤 Karaoke! Join on your phone with the code " + n.JoinCode + " and pick what you sing."
		sub := "Code " + n.JoinCode
		if when != "" {
			sub = when + " · " + sub
		}
		lines := []string{}
		if n.Place != "" {
			lines = append(lines, "📍 "+n.Place)
		}
		if n.About != "" {
			lines = append(lines, clip(n.About, 160))
		}
		card = map[string]any{"title": n.Title, "subtitle": sub, "lines": lines, "icon": "🎤", "url": s.joinURL(n.JoinCode), "app": "karaoke", "kind": "card"}
	}
	st := s.settingsOf(ctx, n.UserID)
	body := map[string]any{"to": firstNonEmpty(strings.TrimSpace(in.To), st.ChatTo, "me"), "text": clip(firstNonEmpty(strings.TrimSpace(in.Text), text), 2000), "card": card,
		"client_id": fmt.Sprintf("karaoke-%d-%d-%s-%d", n.UserID, n.ID, in.What, time.Now().Unix()/30)}
	res, err := postJSONTo(ctx, sib, "/api/v1/send", body)
	if err != nil {
		s.appFail(w, appChat, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": linkTo(sib, "/c/"+str(res, "conversation_id")), "text": "Sent to Pocket Chat"})
}

// nightResultLines are the top scores and the most cheered, in words.
func nightResultLines(perfs []store.Performance) []string {
	type row struct {
		who, what string
		score     int
	}
	var scored []row
	cheered := store.Performance{}
	for _, p := range perfs {
		if p.Score != nil {
			scored = append(scored, row{p.Singer, p.Title, *p.Score})
		}
		if p.Cheers > cheered.Cheers {
			cheered = p
		}
	}
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	lines := []string{}
	medals := []string{"🥇", "🥈", "🥉"}
	for i, r := range scored {
		if i >= 3 {
			break
		}
		lines = append(lines, fmt.Sprintf("%s %s — %s (%d)", medals[i], r.who, r.what, r.score))
	}
	if cheered.Cheers > 0 {
		lines = append(lines, fmt.Sprintf("👏 Most cheered: %s — %s (%d cheers)", cheered.Singer, cheered.Title, cheered.Cheers))
	}
	if len(lines) == 0 {
		for i, p := range perfs {
			if i >= 5 {
				break
			}
			lines = append(lines, "🎵 "+p.Singer+" — "+p.Title)
		}
	}
	return lines
}

// ---- Pocket Contacts ----

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appContacts, scopeContactsRead)
	if !ok {
		return
	}
	q := url.Values{"limit": {"40"}}
	if t := strings.TrimSpace(r.URL.Query().Get("q")); t != "" {
		q.Set("q", t)
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/contacts", q)
	if err != nil {
		s.appFail(w, appContacts, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "contacts") {
		name := firstNonEmpty(str(m, "fn"), str(m, "name"), joinNonEmpty(" ", str(m, "given"), str(m, "family")))
		if name == "" {
			continue
		}
		out = append(out, map[string]any{"id": idOf(m), "name": name, "nickname": str(m, "nickname")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": out})
}

// ---- Pocket Translate ----

// translateLyrics translates a song's lines (kept in the cache until the words change).
func (s *Server) translateLyrics(ctx context.Context, uid int64, so *store.Song, to string) ([]string, error) {
	if to == "" || len(to) > 12 {
		return nil, &userErr{"pick the language"}
	}
	lines, _ := parseLyrics(so.Lyrics)
	if len(lines) == 0 {
		return []string{}, nil
	}
	h := sha256.Sum256([]byte(so.Lyrics))
	key := fmt.Sprintf("tr:%d:%d:%s:%s", uid, so.ID, to, hex.EncodeToString(h[:8]))
	if v, _, ok := s.store.CacheGet(ctx, key); ok {
		var out []string
		if json.Unmarshal([]byte(v), &out) == nil && len(out) == len(lines) {
			return out, nil
		}
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appTranslate, scopeTranslate)
	if err != nil {
		return nil, err
	}
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = l.Text
	}
	// the words that are the same line after line are translated once
	uniq, at := []string{}, map[string]int{}
	for _, t := range texts {
		if _, ok := at[t]; !ok && strings.TrimSpace(t) != "" {
			at[t] = len(uniq)
			uniq = append(uniq, t)
		}
	}
	done := make([]string, 0, len(uniq))
	for start := 0; start < len(uniq); start += 150 {
		end := min(start+150, len(uniq))
		from := firstNonEmpty(so.Language, "auto")
		res, err := postJSONTo(ctx, sib, "/api/v1/translate", map[string]any{"texts": uniq[start:end], "to": to, "from": from})
		if err != nil {
			return nil, err
		}
		arr, _ := res["texts"].([]any)
		if len(arr) != end-start {
			return nil, fmt.Errorf("Pocket Translate gave back a different number of lines")
		}
		for _, x := range arr {
			v, _ := x.(string)
			done = append(done, v)
		}
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		if j, ok := at[t]; ok && j < len(done) {
			out[i] = done[j]
		}
	}
	b, _ := json.Marshal(out)
	_ = s.store.CachePut(ctx, key, string(b))
	return out, nil
}

func (s *Server) handleSongTranslation(w http.ResponseWriter, r *http.Request) {
	so, ok := s.songOf(w, r)
	if !ok {
		return
	}
	to := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("to")), s.settingsOf(r.Context(), so.UserID).TranslateTo)
	lines, err := s.translateLyrics(r.Context(), so.UserID, so, to)
	if err != nil {
		s.appFail(w, appTranslate, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines, "to": to})
}

// ---- Pocket Habits ----

func (s *Server) handleHabitsList(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appHabits, scopeHabitsRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/habits", nil)
	if err != nil {
		s.appFail(w, appHabits, err)
		return
	}
	out := []map[string]any{}
	for _, h := range rows(v, "habits") {
		out = append(out, map[string]any{"id": num(h, "id"), "name": str(h, "name"), "color": str(h, "color")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"habits": out})
}

// tickHabit marks today done for the person's singing habit in Pocket Habits.
func (s *Server) tickHabit(ctx context.Context, uid int64) bool {
	st := s.settingsOf(ctx, uid)
	if st.HabitID <= 0 {
		return false
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appHabits, scopeEntriesWrite)
	if err != nil {
		return false
	}
	var res map[string]any
	if err := sib.client.Do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/habits/%d/entries/%s", st.HabitID, today()), nil, map[string]any{"status": 1}, &res); err != nil {
		s.log.Debug("pocket habits: tick failed", "err", err)
		return false
	}
	return true
}

// ---- Pocket Alerts ----

func (s *Server) alert(ctx context.Context, uid int64, title, body, path, priority string) bool {
	sib, err := s.clients(ctx, uid).conn(ctx, appAlerts, scopeAlertsWrite)
	if err != nil {
		return false
	}
	_, err = postJSONTo(ctx, sib, "/api/v1/alerts", map[string]any{"title": clip(title, 200), "body": clip(body, 2000), "priority": priority, "tags": []string{"microphone"}, "click": s.link(path)})
	if err != nil {
		s.log.Debug("pocket alerts: send failed", "err", err)
		return false
	}
	return true
}
