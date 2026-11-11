package server

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// Songs, covers and recordings are files in DATA_DIR/media/<user id>/ —
// never in the database — so a big video does not slow every query and a
// backup of the folder is a plain copy. The database keeps the relative path.

func (s *Server) mediaRoot() string { return filepath.Join(s.cfg.DataDir, "media") }

// mediaPath is the full path of a relative media name; "" when it would leave the folder.
func (s *Server) mediaPath(rel string) string {
	if rel == "" || strings.Contains(rel, "..") || filepath.IsAbs(rel) {
		return ""
	}
	return filepath.Join(s.mediaRoot(), filepath.FromSlash(rel))
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// newMediaName is a fresh relative name for one person's file.
func newMediaName(uid int64, ext string) string {
	ext = strings.ToLower(ext)
	if !regexp.MustCompile(`^\.[a-z0-9]{1,5}$`).MatchString(ext) {
		ext = ".bin"
	}
	return fmt.Sprintf("%d/%s%s", uid, randomHex(10), ext)
}

// saveMedia copies r into a new file of the person's; it returns the
// relative name and the size. At most max bytes.
func (s *Server) saveMedia(uid int64, ext string, r io.Reader, max int64) (string, int64, error) {
	rel := newMediaName(uid, ext)
	p := s.mediaPath(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", 0, err
	}
	f, err := os.OpenFile(p+".part", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, err
	}
	n, err := io.Copy(f, io.LimitReader(r, max+1))
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err == nil && n > max {
		err = &userErr{fmt.Sprintf("a file can be up to %d MB", max>>20)}
	}
	if err == nil && n == 0 {
		err = &userErr{"the file is empty"}
	}
	if err != nil {
		_ = os.Remove(p + ".part")
		return "", 0, err
	}
	return rel, n, os.Rename(p+".part", p)
}

// moveIntoMedia moves a finished file (a download) into the person's folder.
func (s *Server) moveIntoMedia(uid int64, src string) (string, int64, error) {
	rel := newMediaName(uid, filepath.Ext(src))
	p := s.mediaPath(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", 0, err
	}
	if err := os.Rename(src, p); err != nil {
		in, err := os.Open(src)
		if err != nil {
			return "", 0, err
		}
		defer in.Close()
		r, n, err := s.saveMedia(uid, filepath.Ext(src), in, 8<<30)
		_ = os.Remove(src)
		return r, n, err
	}
	fi, _ := os.Stat(p)
	var n int64
	if fi != nil {
		n = fi.Size()
	}
	return rel, n, nil
}

// dropMedia removes files nothing points at any more.
func (s *Server) dropMedia(ctx context.Context, rels ...string) {
	for _, rel := range rels {
		if rel == "" || s.store.MediaInUse(ctx, rel) {
			continue
		}
		if p := s.mediaPath(rel); p != "" {
			_ = os.Remove(p)
		}
	}
}

// serveMediaFile sends a media file with ranges (seeking in a song).
func (s *Server) serveMediaFile(w http.ResponseWriter, r *http.Request, rel, mime, name string) {
	p := s.mediaPath(rel)
	f, err := os.Open(p)
	if p == "" || err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if mime == "" {
		mime = mimeByExt(rel)
	}
	if mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if name != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, strings.ReplaceAll(safeName(name), `"`, "")))
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

var mediaExts = map[string]string{
	".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".aac": "audio/aac", ".ogg": "audio/ogg", ".oga": "audio/ogg", ".opus": "audio/ogg", ".flac": "audio/flac",
	".wav": "audio/wav", ".weba": "audio/webm", ".mp4": "video/mp4", ".m4v": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime", ".mkv": "video/x-matroska",
	".cdg": "application/octet-stream", ".lrc": "text/plain", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp",
}

func mimeByExt(name string) string { return mediaExts[strings.ToLower(filepath.Ext(name))] }

func isAudioExt(ext string) bool {
	return strings.HasPrefix(mediaExts[strings.ToLower(ext)], "audio/")
}

func isVideoExt(ext string) bool {
	return strings.HasPrefix(mediaExts[strings.ToLower(ext)], "video/")
}

func isPictureExt(ext string) bool {
	return strings.HasPrefix(mediaExts[strings.ToLower(ext)], "image/")
}

func safeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, n)
	return firstNonEmpty(clip(strings.TrimSpace(n), 160), "file")
}

var (
	reInStyle      = regexp.MustCompile(`(?i)[\(\[]?\s*\bin the style of\s+([^\)\]]+?)\s*[\)\]]?\s*$`)
	reTrailKaraoke = regexp.MustCompile(`(?i)\s*[-|–]?\s*\b(karaoke( version)?|instrumental( version)?|minus one)\s*$`)
)

// titleFromFile guesses "Artist - Title (Karaoke)" from a file name.
func titleFromFile(name string) (title, artist string, karaoke bool) {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	base = strings.ReplaceAll(base, "_", " ")
	low := strings.ToLower(base)
	karaoke = strings.Contains(low, "karaoke") || strings.Contains(low, "instrumental") || strings.Contains(low, "backing track")
	// "Song in the Style of Artist" (karaoke channels): the artist is named there
	styleArtist := ""
	if t := reTrailKaraoke.ReplaceAllString(base, ""); t != base {
		base, karaoke = t, true
	}
	if m := reInStyle.FindStringSubmatch(base); m != nil {
		styleArtist = strings.Trim(strings.TrimSpace(m[1]), `"'“”`)
		base = strings.TrimSpace(base[:strings.Index(base, m[0])])
		karaoke = true
	}
	base = regexp.MustCompile(`(?i)\s*[\(\[][^\)\]]*(karaoke|instrumental|backing|lyrics?|official|video|audio|hd|hq)[^\)\]]*[\)\]]`).ReplaceAllString(base, "")
	base = regexp.MustCompile(`^\s*(\d{1,3}[\s.\-_]+)`).ReplaceAllString(base, "")
	base = strings.Join(strings.Fields(base), " ")
	for _, sep := range []string{" - ", " – ", " — ", " | "} {
		if i := strings.Index(base, sep); i > 0 {
			return strings.TrimSpace(base[i+len(sep):]), firstNonEmpty(styleArtist, strings.TrimSpace(base[:i])), karaoke
		}
	}
	return strings.TrimSpace(base), styleArtist, karaoke
}

// ---- ffprobe: length and tags, when the box has it ----

type probeInfo struct {
	DurationMs int64
	Title      string
	Artist     string
	Language   string
	Video      bool
	Lyrics     string
}

func (s *Server) probe(ctx context.Context, path string) probeInfo {
	var out probeInfo
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, bin, "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path).Output()
	if err != nil {
		return out
	}
	var v struct {
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			CodecType   string            `json:"codec_type"`
			Disposition map[string]int    `json:"disposition"`
			Tags        map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if json.Unmarshal(b, &v) != nil {
		return out
	}
	if f, err := strconv.ParseFloat(v.Format.Duration, 64); err == nil {
		out.DurationMs = int64(f * 1000)
	}
	tag := func(names ...string) string {
		for k, val := range v.Format.Tags {
			for _, n := range names {
				if strings.EqualFold(k, n) {
					return strings.TrimSpace(val)
				}
			}
		}
		return ""
	}
	out.Title, out.Artist, out.Language = tag("title"), tag("artist", "album_artist"), tag("language")
	out.Lyrics = tag("lyrics", "unsyncedlyrics", "lyrics-eng")
	for _, st := range v.Streams {
		if st.CodecType == "video" && st.Disposition["attached_pic"] == 0 {
			out.Video = true
		}
	}
	return out
}

// coverFromMedia pulls the picture out of a song file (ffmpeg), "" when none.
func (s *Server) coverFromMedia(ctx context.Context, uid int64, path string, video bool) string {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rel := newMediaName(uid, ".jpg")
	dst := s.mediaPath(rel)
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	args := []string{"-v", "quiet", "-y", "-i", path, "-an", "-frames:v", "1", "-vf", "scale='min(640,iw)':-2", dst}
	if video {
		args = []string{"-v", "quiet", "-y", "-ss", "8", "-i", path, "-an", "-frames:v", "1", "-vf", "scale='min(640,iw)':-2", dst}
	}
	if err := exec.CommandContext(ctx, bin, args...).Run(); err != nil {
		_ = os.Remove(dst)
		return ""
	}
	if fi, err := os.Stat(dst); err != nil || fi.Size() == 0 {
		_ = os.Remove(dst)
		return ""
	}
	return rel
}

// ---- adding songs from files ----

// incomingFile is one file of an upload (or from Pocket Files).
type incomingFile struct {
	name string
	open func() (io.ReadCloser, error)
	size int64
}

const maxSongBytes = 2 << 30

// songFromFiles makes a song from what came in: one audio or video file and,
// beside it, a .cdg (graphics), a .lrc (the words) and a picture. A .zip
// holding these is opened first.
func (s *Server) songFromFiles(ctx context.Context, uid int64, files []incomingFile, source, ref string) (*store.Song, error) {
	var media, cdg, lrc, pic *incomingFile
	var zips []incomingFile
	for i := range files {
		f := &files[i]
		ext := strings.ToLower(filepath.Ext(f.name))
		switch {
		case ext == ".zip":
			zips = append(zips, *f)
		case ext == ".cdg":
			cdg = f
		case ext == ".lrc" || ext == ".txt":
			lrc = f
		case isPictureExt(ext):
			pic = f
		case isAudioExt(ext) || isVideoExt(ext):
			if media == nil {
				media = f
			}
		}
	}
	var cleanup []string
	defer func() {
		for _, p := range cleanup {
			_ = os.Remove(p)
		}
	}()
	for _, z := range zips {
		inner, tmp, err := unzipSong(z, s.cfg.DataDir)
		if err != nil {
			return nil, err
		}
		cleanup = append(cleanup, tmp...)
		for i := range inner {
			f := &inner[i]
			ext := strings.ToLower(filepath.Ext(f.name))
			switch {
			case ext == ".cdg" && cdg == nil:
				cdg = f
			case (ext == ".lrc") && lrc == nil:
				lrc = f
			case isPictureExt(ext) && pic == nil:
				pic = f
			case (isAudioExt(ext) || isVideoExt(ext)) && media == nil:
				media = f
			}
		}
	}
	if media == nil {
		return nil, &userErr{"there is no song in it: send an MP3, M4A, OGG, FLAC, WAV, MP4 or WebM file (with a .cdg or .lrc beside it if you have one), or a .zip of those"}
	}
	so := &store.Song{UserID: uid, Source: source, SourceRef: ref, SourceApp: sourceOf(ctx)}
	var saved []string
	fail := func(err error) (*store.Song, error) {
		for _, rel := range saved {
			if p := s.mediaPath(rel); p != "" {
				_ = os.Remove(p)
			}
		}
		return nil, err
	}
	put := func(f *incomingFile, max int64) (string, int64, error) {
		rc, err := f.open()
		if err != nil {
			return "", 0, err
		}
		defer rc.Close()
		rel, n, err := s.saveMedia(uid, filepath.Ext(f.name), rc, max)
		if err == nil {
			saved = append(saved, rel)
		}
		return rel, n, err
	}
	rel, n, err := put(media, maxSongBytes)
	if err != nil {
		return fail(err)
	}
	ext := filepath.Ext(media.name)
	so.Media, so.MediaSize, so.MediaMime, so.IsVideo = rel, n, mimeByExt(ext), isVideoExt(ext)
	so.Title, so.Artist, _ = titleFromFile(media.name)
	so.Kind = "karaoke"
	if _, _, k := titleFromFile(media.name); !k && cdg == nil {
		so.Kind = "original"
	}
	if cdg != nil {
		r2, _, err := put(cdg, 200<<20)
		if err != nil {
			return fail(err)
		}
		so.CDG, so.Kind = r2, "karaoke"
	}
	if pic != nil {
		if r3, _, err := put(pic, 20<<20); err == nil {
			so.Cover = r3
		}
	}
	if lrc != nil {
		if rc, err := lrc.open(); err == nil {
			b, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
			rc.Close()
			txt := cleanLyrics(string(b))
			if txt != "" {
				so.Lyrics, so.LyricsSynced, so.LyricsSource = txt, isSynced(txt), "file"
				if t, a := lrcTags(txt); t != "" && so.Title == "" {
					so.Title, so.Artist = t, firstNonEmpty(a, so.Artist)
				}
			}
		}
	}
	full := s.mediaPath(so.Media)
	pi := s.probe(ctx, full)
	so.DurationMs = pi.DurationMs
	if pi.Video {
		so.IsVideo = true
	}
	if pi.Title != "" && !strings.Contains(strings.ToLower(pi.Title), "track") {
		so.Title = pi.Title
		if pi.Artist != "" {
			so.Artist = pi.Artist
		}
	} else if so.Artist == "" && pi.Artist != "" {
		so.Artist = pi.Artist
	}
	so.Language = cleanLang(pi.Language)
	if so.Lyrics == "" && pi.Lyrics != "" {
		so.Lyrics, so.LyricsSynced, so.LyricsSource = cleanLyrics(pi.Lyrics), isSynced(pi.Lyrics), "file"
	}
	if so.Cover == "" && so.CDG == "" {
		if c := s.coverFromMedia(ctx, uid, full, so.IsVideo); c != "" {
			so.Cover = c
			saved = append(saved, c)
		}
	}
	if so.IsVideo && so.CDG == "" && so.Lyrics == "" && so.Kind == "karaoke" {
		so.VideoWords = true // a karaoke video carries its words
	}
	so.Title = firstNonEmpty(cleanText(so.Title, maxName), "Untitled song")
	so.Artist = cleanText(so.Artist, maxName)
	if so.Kind == "original" {
		so.VoiceCut = s.settingsOf(ctx, uid).DefaultVoice
	}
	if err := s.store.CreateSong(ctx, so); err != nil {
		return fail(err)
	}
	return so, nil
}

func cleanLang(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) == 3 {
		if m := map[string]string{"eng": "en", "srp": "sr", "hrv": "hr", "bos": "bs", "deu": "de", "ger": "de", "fra": "fr", "fre": "fr", "spa": "es", "ita": "it", "por": "pt", "rus": "ru"}[v]; m != "" {
			return m
		}
		if v == "und" {
			return ""
		}
	}
	if len(v) > 8 {
		return ""
	}
	return v
}

// unzipSong opens a karaoke .zip into temporary files.
func unzipSong(z incomingFile, dataDir string) ([]incomingFile, []string, error) {
	rc, err := z.open()
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()
	tmpDir := filepath.Join(dataDir, "tmp")
	_ = os.MkdirAll(tmpDir, 0o755)
	tf, err := os.CreateTemp(tmpDir, "zip-*")
	if err != nil {
		return nil, nil, err
	}
	tmp := []string{tf.Name()}
	if _, err := io.Copy(tf, io.LimitReader(rc, maxSongBytes)); err != nil {
		tf.Close()
		return nil, tmp, err
	}
	tf.Close()
	zr, err := zip.OpenReader(tf.Name())
	if err != nil {
		return nil, tmp, &userErr{"the .zip could not be opened"}
	}
	defer zr.Close()
	var out []incomingFile
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || strings.HasPrefix(filepath.Base(f.Name), ".") || strings.Contains(f.Name, "__MACOSX") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if mediaExts[ext] == "" {
			continue
		}
		in, err := f.Open()
		if err != nil {
			continue
		}
		of, err := os.CreateTemp(tmpDir, "part-*"+ext)
		if err != nil {
			in.Close()
			continue
		}
		_, err = io.Copy(of, io.LimitReader(in, maxSongBytes))
		in.Close()
		of.Close()
		tmp = append(tmp, of.Name())
		if err != nil {
			continue
		}
		p := of.Name()
		out = append(out, incomingFile{name: filepath.Base(f.Name), open: func() (io.ReadCloser, error) { return os.Open(p) }})
		if len(out) > 20 {
			break
		}
	}
	return out, tmp, nil
}

// handleUploadSong takes a song from the browser (multipart "file", several allowed).
func (s *Server) handleUploadSong(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "send the song as a form upload")
		return
	}
	tmpDir := filepath.Join(s.cfg.DataDir, "tmp")
	_ = os.MkdirAll(tmpDir, 0o755)
	var files []incomingFile
	var tmps []string
	defer func() {
		for _, p := range tmps {
			_ = os.Remove(p)
		}
	}()
	fields := map[string]string{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the upload broke off")
			return
		}
		if part.FileName() == "" {
			b, _ := io.ReadAll(io.LimitReader(part, 4096))
			fields[part.FormName()] = string(b)
			continue
		}
		name := safeName(part.FileName())
		tf, err := os.CreateTemp(tmpDir, "up-*"+strings.ToLower(filepath.Ext(name)))
		if err != nil {
			s.fail(w, err, "upload")
			return
		}
		tmps = append(tmps, tf.Name())
		n, err := io.Copy(tf, io.LimitReader(part, maxSongBytes+1))
		tf.Close()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the upload broke off")
			return
		}
		if n > maxSongBytes {
			writeErr(w, http.StatusBadRequest, "a file can be up to 2 GB")
			return
		}
		p := tf.Name()
		files = append(files, incomingFile{name: name, size: n, open: func() (io.ReadCloser, error) { return os.Open(p) }})
		if len(files) > 10 {
			break
		}
	}
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, "no file")
		return
	}
	so, err := s.songFromFiles(ctx, u.ID, files, "upload", "")
	if err != nil {
		s.userFail(w, err, "upload")
		return
	}
	if t := strings.TrimSpace(fields["title"]); t != "" {
		so.Title = cleanText(t, maxName)
	}
	if a := strings.TrimSpace(fields["artist"]); a != "" {
		so.Artist = cleanText(a, maxName)
	}
	if k := fields["kind"]; k == "karaoke" || k == "original" {
		so.Kind = k
	}
	_ = s.store.UpdateSong(ctx, so)
	s.afterSongAdded(ctx, u.ID, so)
	writeJSON(w, http.StatusCreated, so)
}

// handleSongMedia plays a song: our own copy, or Pocket Music's through the link.
func (s *Server) handleSongMedia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	so, err := s.store.GetSong(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.playSong(w, r, so)
}

func (s *Server) playSong(w http.ResponseWriter, r *http.Request, so *store.Song) {
	if so.Media != "" {
		s.serveMediaFile(w, r, so.Media, so.MediaMime, "")
		return
	}
	if so.Source == "music" && so.SourceRef != "" {
		s.streamFromMusic(w, r, so)
		return
	}
	writeErr(w, http.StatusNotFound, "this song plays from YouTube")
}

func (s *Server) handleSongCover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	so, err := s.store.GetSong(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.serveCover(w, r, so)
}

func (s *Server) serveCover(w http.ResponseWriter, r *http.Request, so *store.Song) {
	if so.Cover != "" {
		s.serveMediaFile(w, r, so.Cover, "", "")
		return
	}
	if so.Source == "music" && so.SourceRef != "" {
		s.coverFromMusic(w, r, so)
		return
	}
	if so.CoverURL != "" {
		http.Redirect(w, r, so.CoverURL, http.StatusFound)
		return
	}
	writeErr(w, http.StatusNotFound, "no picture")
}

func (s *Server) handleSongCDG(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	so, err := s.store.GetSong(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil || so.CDG == "" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.serveMediaFile(w, r, so.CDG, "application/octet-stream", "")
}

// handleSongCoverUpload sets a song's picture.
func (s *Server) handleSongCoverUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	so, err := s.store.GetSong(ctx, u.ID, idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "a picture can be up to 20 MB")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no file")
		return
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	mime := http.DetectContentType(head[:n])
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[strings.Split(mime, ";")[0]]
	if ext == "" {
		writeErr(w, http.StatusBadRequest, "a JPEG, PNG or WebP picture")
		return
	}
	_ = hdr
	rel, _, err := s.saveMedia(u.ID, ext, io.MultiReader(strings.NewReader(string(head[:n])), f), 20<<20)
	if err != nil {
		s.userFail(w, err, "picture")
		return
	}
	old := so.Cover
	so.Cover = rel
	if err := s.store.UpdateSong(ctx, so); err != nil {
		s.fail(w, err, "picture")
		return
	}
	s.dropMedia(ctx, old)
	writeJSON(w, http.StatusOK, so)
}
