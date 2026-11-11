package store

import (
	"context"
	"database/sql"
	"strings"
)

// Song is one song in a person's karaoke library.
type Song struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"-"`
	Title          string  `json:"title"`
	Artist         string  `json:"artist"`
	Language       string  `json:"language"`
	Kind           string  `json:"kind"`
	Source         string  `json:"source"`
	SourceRef      string  `json:"source_ref"`
	Media          string  `json:"-"`
	MediaMime      string  `json:"media_mime"`
	MediaSize      int64   `json:"media_size"`
	IsVideo        bool    `json:"is_video"`
	CDG            string  `json:"-"`
	Cover          string  `json:"-"`
	CoverURL       string  `json:"cover_url"`
	DurationMs     int64   `json:"duration_ms"`
	Lyrics         string  `json:"lyrics"`
	LyricsSynced   bool    `json:"lyrics_synced"`
	LyricsSource   string  `json:"lyrics_source"`
	LyricsOffsetMs int64   `json:"lyrics_offset_ms"`
	VideoWords     bool    `json:"video_words"`
	KeyShift       int     `json:"key_shift"`
	VoiceCut       int     `json:"voice_cut"`
	Tags           string  `json:"tags"`
	Notes          string  `json:"notes"`
	Favourite      bool    `json:"favourite"`
	Plays          int     `json:"plays"`
	LastSungAt     *string `json:"last_sung_at"`
	CopyState      string  `json:"copy_state"`
	CopyError      string  `json:"copy_error"`
	CopyProgress   int     `json:"copy_progress"`
	AddedBy        string  `json:"added_by"`
	SourceApp      string  `json:"source_app"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`

	// worked out for the page
	HasMedia bool    `json:"has_media"` // there is our own copy (key change, voice cut and recording work)
	HasCDG   bool    `json:"has_cdg"`
	HasCover bool    `json:"has_cover"`
	Best     *int    `json:"best,omitempty"` // best score
	Books    []int64 `json:"books,omitempty"`
}

const songCols = `id, user_id, title, artist, language, kind, source, source_ref, media, media_mime, media_size, is_video, cdg, cover, cover_url,
 duration_ms, lyrics, lyrics_synced, lyrics_source, lyrics_offset_ms, video_words, key_shift, voice_cut, tags, notes, favourite, plays, last_sung_at,
 copy_state, copy_error, copy_progress, added_by, source_app, created_at, updated_at`

type scanner interface{ Scan(...any) error }

func scanSong(r scanner) (*Song, error) {
	var x Song
	var vid, syn, vw, fav int
	if err := r.Scan(&x.ID, &x.UserID, &x.Title, &x.Artist, &x.Language, &x.Kind, &x.Source, &x.SourceRef, &x.Media, &x.MediaMime, &x.MediaSize, &vid, &x.CDG, &x.Cover, &x.CoverURL,
		&x.DurationMs, &x.Lyrics, &syn, &x.LyricsSource, &x.LyricsOffsetMs, &vw, &x.KeyShift, &x.VoiceCut, &x.Tags, &x.Notes, &fav, &x.Plays, &x.LastSungAt,
		&x.CopyState, &x.CopyError, &x.CopyProgress, &x.AddedBy, &x.SourceApp, &x.CreatedAt, &x.UpdatedAt); err != nil {
		return nil, err
	}
	x.IsVideo, x.LyricsSynced, x.VideoWords, x.Favourite = vid == 1, syn == 1, vw == 1, fav == 1
	x.HasMedia, x.HasCDG, x.HasCover = x.Media != "", x.CDG != "", x.Cover != ""
	return &x, nil
}

func collectSongs(rows *sql.Rows, err error) ([]Song, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Song{}
	for rows.Next() {
		x, err := scanSong(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// SongFilter narrows the song list.
type SongFilter struct {
	Q         string
	Source    string
	Language  string
	Favourite bool
	NoWords   bool // no words yet
	BookID    int64
	Sort      string // title | artist | added | sung | plays
	Limit     int
	Offset    int
}

// ListSongs returns one person's songs.
func (s *Store) ListSongs(ctx context.Context, uid int64, f SongFilter) ([]Song, int, error) {
	where := []string{"s.user_id = ?"}
	args := []any{uid}
	if q := strings.TrimSpace(f.Q); q != "" {
		for _, w := range strings.Fields(strings.ToLower(q)) {
			where = append(where, "(lower(s.title) LIKE ? OR lower(s.artist) LIKE ? OR lower(s.tags) LIKE ?)")
			p := "%" + w + "%"
			args = append(args, p, p, p)
		}
	}
	if f.Source != "" {
		where = append(where, "s.source = ?")
		args = append(args, f.Source)
	}
	if f.Language != "" {
		where = append(where, "s.language = ?")
		args = append(args, f.Language)
	}
	if f.Favourite {
		where = append(where, "s.favourite = 1")
	}
	if f.NoWords {
		where = append(where, "s.lyrics = '' AND s.video_words = 0 AND s.cdg = ''")
	}
	from := "songs s"
	if f.BookID > 0 {
		from = "songs s JOIN book_songs b ON b.song_id = s.id AND b.book_id = ?"
		args = append([]any{f.BookID}, args...)
	}
	order := "lower(s.title)"
	switch f.Sort {
	case "artist":
		order = "lower(s.artist), lower(s.title)"
	case "added":
		order = "s.created_at DESC"
	case "sung":
		order = "COALESCE(s.last_sung_at, '') DESC, lower(s.title)"
	case "plays":
		order = "s.plays DESC, lower(s.title)"
	case "book":
		if f.BookID > 0 {
			order = "b.position, lower(s.title)"
		}
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+from+` WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	lim := f.Limit
	if lim <= 0 || lim > 1000 {
		lim = 200
	}
	cols := "s." + strings.ReplaceAll(strings.ReplaceAll(songCols, "\n", ""), ", ", ", s.")
	list, err := collectSongs(s.DB.QueryContext(ctx, `SELECT `+cols+` FROM `+from+` WHERE `+cond+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, lim, f.Offset)...))
	if err != nil {
		return nil, 0, err
	}
	best, err := s.bestScores(ctx, uid)
	if err == nil {
		for i := range list {
			if b, ok := best[list[i].ID]; ok {
				v := b
				list[i].Best = &v
			}
		}
	}
	return list, total, nil
}

func (s *Store) bestScores(ctx context.Context, uid int64) (map[int64]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT song_id, MAX(score) FROM performances WHERE user_id = ? AND song_id IS NOT NULL AND score IS NOT NULL GROUP BY song_id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var b int
		if rows.Scan(&id, &b) == nil {
			out[id] = b
		}
	}
	return out, rows.Err()
}

// GetSong returns one of the person's songs.
func (s *Store) GetSong(ctx context.Context, uid, id int64) (*Song, error) {
	x, err := scanSong(s.DB.QueryRowContext(ctx, `SELECT `+songCols+` FROM songs WHERE id = ? AND user_id = ?`, id, uid))
	if err != nil {
		return nil, notFound(err)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT book_id FROM book_songs WHERE song_id = ? AND user_id = ?`, id, uid)
	if err == nil {
		for rows.Next() {
			var b int64
			if rows.Scan(&b) == nil {
				x.Books = append(x.Books, b)
			}
		}
		rows.Close()
	}
	return x, nil
}

// SongBySource finds a song the person already has from the same place.
func (s *Store) SongBySource(ctx context.Context, uid int64, source, ref string) (*Song, error) {
	if ref == "" {
		return nil, ErrNotFound
	}
	x, err := scanSong(s.DB.QueryRowContext(ctx, `SELECT `+songCols+` FROM songs WHERE user_id = ? AND source = ? AND source_ref = ? ORDER BY id LIMIT 1`, uid, source, ref))
	return x, notFound(err)
}

// SongsWithCopyState lists the songs waiting for (or working on) a copy, box-wide.
func (s *Store) SongsWithCopyState(ctx context.Context, states ...string) ([]Song, error) {
	if len(states) == 0 {
		return nil, nil
	}
	q := strings.TrimSuffix(strings.Repeat("?,", len(states)), ",")
	args := make([]any, len(states))
	for i, v := range states {
		args[i] = v
	}
	return collectSongs(s.DB.QueryContext(ctx, `SELECT `+songCols+` FROM songs WHERE copy_state IN (`+q+`) ORDER BY updated_at`, args...))
}

// CreateSong adds a song.
func (s *Store) CreateSong(ctx context.Context, x *Song) error {
	now := Now()
	if x.Kind == "" {
		x.Kind = "karaoke"
	}
	if x.Source == "" {
		x.Source = "upload"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO songs (user_id, title, artist, language, kind, source, source_ref, media, media_mime, media_size, is_video, cdg, cover, cover_url,
 duration_ms, lyrics, lyrics_synced, lyrics_source, lyrics_offset_ms, video_words, key_shift, voice_cut, tags, notes, favourite, plays, last_sung_at,
 copy_state, copy_error, copy_progress, added_by, source_app, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		x.UserID, x.Title, x.Artist, x.Language, x.Kind, x.Source, x.SourceRef, x.Media, x.MediaMime, x.MediaSize, boolInt(x.IsVideo), x.CDG, x.Cover, x.CoverURL,
		x.DurationMs, x.Lyrics, boolInt(x.LyricsSynced), x.LyricsSource, x.LyricsOffsetMs, boolInt(x.VideoWords), x.KeyShift, x.VoiceCut, x.Tags, x.Notes, boolInt(x.Favourite), x.Plays, x.LastSungAt,
		x.CopyState, x.CopyError, x.CopyProgress, x.AddedBy, x.SourceApp, firstNon(x.CreatedAt, now), now)
	if err != nil {
		return err
	}
	x.ID, _ = res.LastInsertId()
	x.CreatedAt, x.UpdatedAt = firstNon(x.CreatedAt, now), now
	x.HasMedia, x.HasCDG, x.HasCover = x.Media != "", x.CDG != "", x.Cover != ""
	return nil
}

// UpdateSong saves every field of a song.
func (s *Store) UpdateSong(ctx context.Context, x *Song) error {
	x.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE songs SET title=?, artist=?, language=?, kind=?, source=?, source_ref=?, media=?, media_mime=?, media_size=?, is_video=?, cdg=?, cover=?, cover_url=?,
 duration_ms=?, lyrics=?, lyrics_synced=?, lyrics_source=?, lyrics_offset_ms=?, video_words=?, key_shift=?, voice_cut=?, tags=?, notes=?, favourite=?, plays=?, last_sung_at=?,
 copy_state=?, copy_error=?, copy_progress=?, added_by=?, updated_at=? WHERE id = ? AND user_id = ?`,
		x.Title, x.Artist, x.Language, x.Kind, x.Source, x.SourceRef, x.Media, x.MediaMime, x.MediaSize, boolInt(x.IsVideo), x.CDG, x.Cover, x.CoverURL,
		x.DurationMs, x.Lyrics, boolInt(x.LyricsSynced), x.LyricsSource, x.LyricsOffsetMs, boolInt(x.VideoWords), x.KeyShift, x.VoiceCut, x.Tags, x.Notes, boolInt(x.Favourite), x.Plays, x.LastSungAt,
		x.CopyState, x.CopyError, x.CopyProgress, x.AddedBy, x.UpdatedAt, x.ID, x.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	x.HasMedia, x.HasCDG, x.HasCover = x.Media != "", x.CDG != "", x.Cover != ""
	return nil
}

// SetCopyProgress updates only the copy fields (the downloader, box-wide).
func (s *Store) SetCopyProgress(ctx context.Context, id int64, state string, progress int, errText string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE songs SET copy_state = ?, copy_progress = ?, copy_error = ?, updated_at = ? WHERE id = ?`, state, progress, errText, Now(), id)
	return err
}

// DeleteSong removes a song (its performances keep the title).
func (s *Store) DeleteSong(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM songs WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MediaInUse reports whether any song (of anyone) still points at a file.
func (s *Store) MediaInUse(ctx context.Context, rel string) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM songs WHERE media = ? OR cdg = ? OR cover = ?`, rel, rel, rel).Scan(&n)
	if n == 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM performances WHERE recording = ?`, rel).Scan(&n)
	}
	return n > 0
}

// Languages lists the languages of a person's songs.
func (s *Store) Languages(ctx context.Context, uid int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT language FROM songs WHERE user_id = ? AND language != '' ORDER BY language`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var l string
		if rows.Scan(&l) == nil {
			out = append(out, l)
		}
	}
	return out, rows.Err()
}

// ---- singers ----

// Singer is someone who sings often.
type Singer struct {
	ID        int64   `json:"id"`
	UserID    int64   `json:"-"`
	Name      string  `json:"name"`
	Colour    string  `json:"colour"`
	Emoji     string  `json:"emoji"`
	ContactID string  `json:"contact_id"`
	Notes     string  `json:"notes"`
	CreatedAt string  `json:"created_at"`
	Sung      int     `json:"sung"`
	Best      *int    `json:"best"`
	Average   float64 `json:"average"`
	LastAt    *string `json:"last_at"`
}

// ListSingers returns a person's singers with how much they sang.
func (s *Store) ListSingers(ctx context.Context, uid int64) ([]Singer, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT g.id, g.name, g.colour, g.emoji, g.contact_id, g.notes, g.created_at,
 (SELECT COUNT(1) FROM performances p WHERE p.singer_id = g.id),
 (SELECT MAX(score) FROM performances p WHERE p.singer_id = g.id),
 COALESCE((SELECT AVG(score) FROM performances p WHERE p.singer_id = g.id AND score IS NOT NULL), 0),
 (SELECT MAX(sung_at) FROM performances p WHERE p.singer_id = g.id)
 FROM singers g WHERE g.user_id = ? ORDER BY lower(g.name)`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Singer{}
	for rows.Next() {
		var x Singer
		var best sql.NullInt64
		if err := rows.Scan(&x.ID, &x.Name, &x.Colour, &x.Emoji, &x.ContactID, &x.Notes, &x.CreatedAt, &x.Sung, &best, &x.Average, &x.LastAt); err != nil {
			return nil, err
		}
		if best.Valid {
			b := int(best.Int64)
			x.Best = &b
		}
		x.UserID = uid
		out = append(out, x)
	}
	return out, rows.Err()
}

// GetSinger returns one of the person's singers.
func (s *Store) GetSinger(ctx context.Context, uid, id int64) (*Singer, error) {
	var x Singer
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, name, colour, emoji, contact_id, notes, created_at FROM singers WHERE id = ? AND user_id = ?`, id, uid).
		Scan(&x.ID, &x.UserID, &x.Name, &x.Colour, &x.Emoji, &x.ContactID, &x.Notes, &x.CreatedAt)
	return &x, notFound(err)
}

// SaveSinger creates or updates a singer.
func (s *Store) SaveSinger(ctx context.Context, x *Singer) error {
	if x.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO singers (user_id, name, colour, emoji, contact_id, notes, created_at) VALUES (?,?,?,?,?,?,?)`,
			x.UserID, x.Name, x.Colour, x.Emoji, x.ContactID, x.Notes, firstNon(x.CreatedAt, Now()))
		if err != nil {
			return err
		}
		x.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE singers SET name=?, colour=?, emoji=?, contact_id=?, notes=? WHERE id = ? AND user_id = ?`, x.Name, x.Colour, x.Emoji, x.ContactID, x.Notes, x.ID, x.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSinger removes a singer (what they sang stays, with the name).
func (s *Store) DeleteSinger(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM singers WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- song books ----

// Book is a list of songs.
type Book struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Name      string `json:"name"`
	About     string `json:"about"`
	CreatedAt string `json:"created_at"`
	Songs     int    `json:"songs"`
}

// ListBooks returns a person's song books.
func (s *Store) ListBooks(ctx context.Context, uid int64) ([]Book, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id, b.name, b.about, b.created_at, (SELECT COUNT(1) FROM book_songs x WHERE x.book_id = b.id) FROM books b WHERE b.user_id = ? ORDER BY lower(b.name)`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Book{}
	for rows.Next() {
		var x Book
		if err := rows.Scan(&x.ID, &x.Name, &x.About, &x.CreatedAt, &x.Songs); err != nil {
			return nil, err
		}
		x.UserID = uid
		out = append(out, x)
	}
	return out, rows.Err()
}

// GetBook returns one of the person's song books.
func (s *Store) GetBook(ctx context.Context, uid, id int64) (*Book, error) {
	var x Book
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, name, about, created_at, (SELECT COUNT(1) FROM book_songs WHERE book_id = books.id) FROM books WHERE id = ? AND user_id = ?`, id, uid).
		Scan(&x.ID, &x.UserID, &x.Name, &x.About, &x.CreatedAt, &x.Songs)
	return &x, notFound(err)
}

// SaveBook creates or renames a song book.
func (s *Store) SaveBook(ctx context.Context, x *Book) error {
	if x.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO books (user_id, name, about, created_at) VALUES (?,?,?,?)`, x.UserID, x.Name, x.About, firstNon(x.CreatedAt, Now()))
		if err != nil {
			return err
		}
		x.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE books SET name=?, about=? WHERE id = ? AND user_id = ?`, x.Name, x.About, x.ID, x.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBook removes a song book (the songs stay).
func (s *Store) DeleteBook(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM books WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetInBook puts a song in a book or takes it out. Both must be the person's.
func (s *Store) SetInBook(ctx context.Context, uid, bookID, songID int64, in bool) error {
	if _, err := s.GetBook(ctx, uid, bookID); err != nil {
		return err
	}
	if _, err := s.GetSong(ctx, uid, songID); err != nil {
		return err
	}
	if !in {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM book_songs WHERE book_id = ? AND song_id = ? AND user_id = ?`, bookID, songID, uid)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO book_songs (book_id, song_id, user_id, position) VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM book_songs WHERE book_id = ?)) ON CONFLICT DO NOTHING`, bookID, songID, uid, bookID)
	return err
}

// BookSongIDs lists a book's song ids in order.
func (s *Store) BookSongIDs(ctx context.Context, uid, bookID int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT song_id FROM book_songs WHERE book_id = ? AND user_id = ? ORDER BY position`, bookID, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func firstNon(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
