package store

import (
	"context"
	"database/sql"
	"strings"
)

// Night is a karaoke night.
type Night struct {
	ID           int64   `json:"id"`
	UserID       int64   `json:"-"`
	Title        string  `json:"title"`
	StartsAt     string  `json:"starts_at"`
	Place        string  `json:"place"`
	About        string  `json:"about"`
	Status       string  `json:"status"`
	JoinCode     string  `json:"join_code"`
	ScreenKey    string  `json:"screen_key"`
	Rotation     string  `json:"rotation"`
	Approve      bool    `json:"approve"`
	GuestYouTube bool    `json:"guest_youtube"`
	PerGuest     int     `json:"per_guest"`
	BookID       *int64  `json:"book_id"`
	Scores       bool    `json:"scores"`
	Autoplay     int     `json:"autoplay"`
	CurrentID    *int64  `json:"current_id"`
	Phase        string  `json:"phase"`
	PhaseAt      *string `json:"phase_at"`
	PositionMs   int64   `json:"position_ms"`
	KeyNow       int     `json:"key_now"`
	VoiceNow     int     `json:"voice_now"`
	Volume       int     `json:"volume"`
	CalendarRef  string  `json:"calendar_ref"`
	StartedAt    *string `json:"started_at"`
	EndedAt      *string `json:"ended_at"`
	SourceApp    string  `json:"source_app"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`

	Sung    int `json:"sung"`
	Waiting int `json:"waiting"`
	Guests  int `json:"guests"`
}

const nightCols = `id, user_id, title, starts_at, place, about, status, join_code, screen_key, rotation, approve, guest_youtube, per_guest, book_id, scores, autoplay,
 current_id, phase, phase_at, position_ms, key_now, voice_now, volume, calendar_ref, started_at, ended_at, source_app, created_at, updated_at,
 (SELECT COUNT(1) FROM queue q WHERE q.night_id = nights.id AND q.status = 'done'),
 (SELECT COUNT(1) FROM queue q WHERE q.night_id = nights.id AND q.status IN ('waiting','asked')),
 (SELECT COUNT(1) FROM guests g WHERE g.night_id = nights.id)`

func scanNight(r scanner) (*Night, error) {
	var x Night
	var ap, gy, sc int
	if err := r.Scan(&x.ID, &x.UserID, &x.Title, &x.StartsAt, &x.Place, &x.About, &x.Status, &x.JoinCode, &x.ScreenKey, &x.Rotation, &ap, &gy, &x.PerGuest, &x.BookID, &sc, &x.Autoplay,
		&x.CurrentID, &x.Phase, &x.PhaseAt, &x.PositionMs, &x.KeyNow, &x.VoiceNow, &x.Volume, &x.CalendarRef, &x.StartedAt, &x.EndedAt, &x.SourceApp, &x.CreatedAt, &x.UpdatedAt,
		&x.Sung, &x.Waiting, &x.Guests); err != nil {
		return nil, err
	}
	x.Approve, x.GuestYouTube, x.Scores = ap == 1, gy == 1, sc == 1
	return &x, nil
}

// ListNights returns a person's nights, the newest first.
func (s *Store) ListNights(ctx context.Context, uid int64) ([]Night, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+nightCols+` FROM nights WHERE user_id = ? ORDER BY CASE status WHEN 'live' THEN 0 WHEN 'planned' THEN 1 ELSE 2 END,
 CASE WHEN status = 'planned' THEN starts_at END ASC, COALESCE(started_at, starts_at, created_at) DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Night{}
	for rows.Next() {
		x, err := scanNight(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// GetNight returns one of the person's nights.
func (s *Store) GetNight(ctx context.Context, uid, id int64) (*Night, error) {
	x, err := scanNight(s.DB.QueryRowContext(ctx, `SELECT `+nightCols+` FROM nights WHERE id = ? AND user_id = ?`, id, uid))
	return x, notFound(err)
}

// NightByID returns a night whoever it belongs to (the live hub, links).
func (s *Store) NightByID(ctx context.Context, id int64) (*Night, error) {
	x, err := scanNight(s.DB.QueryRowContext(ctx, `SELECT `+nightCols+` FROM nights WHERE id = ?`, id))
	return x, notFound(err)
}

// NightByCode finds a night by the code guests type (any case).
func (s *Store) NightByCode(ctx context.Context, code string) (*Night, error) {
	x, err := scanNight(s.DB.QueryRowContext(ctx, `SELECT `+nightCols+` FROM nights WHERE join_code = ?`, strings.ToUpper(strings.TrimSpace(code))))
	return x, notFound(err)
}

// NightByScreen finds a night by its big screen's key.
func (s *Store) NightByScreen(ctx context.Context, key string) (*Night, error) {
	if len(key) < 16 {
		return nil, ErrNotFound
	}
	x, err := scanNight(s.DB.QueryRowContext(ctx, `SELECT `+nightCols+` FROM nights WHERE screen_key = ?`, key))
	return x, notFound(err)
}

// NightsByStatus lists nights in one state, box-wide (background work).
func (s *Store) NightsByStatus(ctx context.Context, status string) ([]Night, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+nightCols+` FROM nights WHERE status = ?`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Night{}
	for rows.Next() {
		x, err := scanNight(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// CreateNight adds a night.
func (s *Store) CreateNight(ctx context.Context, x *Night) error {
	now := Now()
	if x.Status == "" {
		x.Status = "planned"
	}
	if x.Phase == "" {
		x.Phase = "idle"
	}
	if x.Volume == 0 {
		x.Volume = 100
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO nights (user_id, title, starts_at, place, about, status, join_code, screen_key, rotation, approve, guest_youtube, per_guest, book_id, scores, autoplay,
 phase, volume, calendar_ref, started_at, ended_at, source_app, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		x.UserID, x.Title, x.StartsAt, x.Place, x.About, x.Status, x.JoinCode, x.ScreenKey, x.Rotation, boolInt(x.Approve), boolInt(x.GuestYouTube), x.PerGuest, nullI(x.BookID), boolInt(x.Scores), x.Autoplay,
		x.Phase, x.Volume, x.CalendarRef, x.StartedAt, x.EndedAt, x.SourceApp, firstNon(x.CreatedAt, now), now)
	if err != nil {
		return conflict(err)
	}
	x.ID, _ = res.LastInsertId()
	return nil
}

// UpdateNight saves a night's details and its screen state.
func (s *Store) UpdateNight(ctx context.Context, x *Night) error {
	x.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE nights SET title=?, starts_at=?, place=?, about=?, status=?, join_code=?, screen_key=?, rotation=?, approve=?, guest_youtube=?, per_guest=?, book_id=?, scores=?, autoplay=?,
 current_id=?, phase=?, phase_at=?, position_ms=?, key_now=?, voice_now=?, volume=?, calendar_ref=?, started_at=?, ended_at=?, updated_at=? WHERE id = ? AND user_id = ?`,
		x.Title, x.StartsAt, x.Place, x.About, x.Status, x.JoinCode, x.ScreenKey, x.Rotation, boolInt(x.Approve), boolInt(x.GuestYouTube), x.PerGuest, nullI(x.BookID), boolInt(x.Scores), x.Autoplay,
		nullI(x.CurrentID), x.Phase, x.PhaseAt, x.PositionMs, x.KeyNow, x.VoiceNow, x.Volume, x.CalendarRef, x.StartedAt, x.EndedAt, x.UpdatedAt, x.ID, x.UserID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteNight removes a night, its queue and its guests (what was sung stays in the history).
func (s *Store) DeleteNight(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM nights WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- guests ----

// Guest is someone who joined a night from their phone.
type Guest struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"-"`
	NightID  int64  `json:"night_id"`
	Name     string `json:"name"`
	SingerID *int64 `json:"singer_id"`
	Blocked  bool   `json:"blocked"`
	JoinedAt string `json:"joined_at"`
	SeenAt   string `json:"seen_at"`
	Sung     int    `json:"sung"`
	Waiting  int    `json:"waiting"`
}

const guestCols = `id, user_id, night_id, name, singer_id, blocked, joined_at, seen_at,
 (SELECT COUNT(1) FROM queue q WHERE q.guest_id = guests.id AND q.status = 'done'),
 (SELECT COUNT(1) FROM queue q WHERE q.guest_id = guests.id AND q.status IN ('waiting','asked','singing'))`

func scanGuest(r scanner) (*Guest, error) {
	var x Guest
	var bl int
	if err := r.Scan(&x.ID, &x.UserID, &x.NightID, &x.Name, &x.SingerID, &bl, &x.JoinedAt, &x.SeenAt, &x.Sung, &x.Waiting); err != nil {
		return nil, err
	}
	x.Blocked = bl == 1
	return &x, nil
}

// CreateGuest adds a guest to a night.
func (s *Store) CreateGuest(ctx context.Context, x *Guest, tokenHash string) error {
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO guests (user_id, night_id, name, token_hash, singer_id, joined_at, seen_at) VALUES (?,?,?,?,?,?,?)`, x.UserID, x.NightID, x.Name, tokenHash, nullI(x.SingerID), now, now)
	if err != nil {
		return conflict(err)
	}
	x.ID, _ = res.LastInsertId()
	x.JoinedAt, x.SeenAt = now, now
	return nil
}

// GuestByToken finds a night's guest by their phone's token hash.
func (s *Store) GuestByToken(ctx context.Context, nightID int64, tokenHash string) (*Guest, error) {
	x, err := scanGuest(s.DB.QueryRowContext(ctx, `SELECT `+guestCols+` FROM guests WHERE night_id = ? AND token_hash = ?`, nightID, tokenHash))
	if err != nil {
		return nil, notFound(err)
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE guests SET seen_at = ? WHERE id = ?`, Now(), x.ID)
	return x, nil
}

// ListGuests returns a night's guests.
func (s *Store) ListGuests(ctx context.Context, uid, nightID int64) ([]Guest, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+guestCols+` FROM guests WHERE night_id = ? AND user_id = ? ORDER BY joined_at`, nightID, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Guest{}
	for rows.Next() {
		x, err := scanGuest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// GetGuest returns one of a person's guests.
func (s *Store) GetGuest(ctx context.Context, uid, id int64) (*Guest, error) {
	x, err := scanGuest(s.DB.QueryRowContext(ctx, `SELECT `+guestCols+` FROM guests WHERE id = ? AND user_id = ?`, id, uid))
	return x, notFound(err)
}

// UpdateGuest renames, links or blocks a guest.
func (s *Store) UpdateGuest(ctx context.Context, x *Guest) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE guests SET name = ?, singer_id = ?, blocked = ? WHERE id = ? AND user_id = ?`, x.Name, nullI(x.SingerID), boolInt(x.Blocked), x.ID, x.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- the queue ----

// Entry is one song in a night's queue.
type Entry struct {
	ID        int64   `json:"id"`
	UserID    int64   `json:"-"`
	NightID   int64   `json:"night_id"`
	SongID    int64   `json:"song_id"`
	Singer    string  `json:"singer"`
	SingerID  *int64  `json:"singer_id"`
	GuestID   *int64  `json:"guest_id"`
	Position  int     `json:"position"`
	Status    string  `json:"status"`
	KeyShift  int     `json:"key_shift"`
	Note      string  `json:"note"`
	CreatedAt string  `json:"created_at"`
	StartedAt *string `json:"started_at"`
	DoneAt    *string `json:"done_at"`

	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Duration int64  `json:"duration_ms"`
	Source   string `json:"source"`
	Ready    bool   `json:"ready"` // can be played (YouTube, a copy, Pocket Music…)
}

const entryCols = `q.id, q.user_id, q.night_id, q.song_id, q.singer, q.singer_id, q.guest_id, q.position, q.status, q.key_shift, q.note, q.created_at, q.started_at, q.done_at,
 s.title, s.artist, s.duration_ms, s.source, s.media, s.source_ref`

func scanEntry(r scanner) (*Entry, error) {
	var x Entry
	var media, ref string
	if err := r.Scan(&x.ID, &x.UserID, &x.NightID, &x.SongID, &x.Singer, &x.SingerID, &x.GuestID, &x.Position, &x.Status, &x.KeyShift, &x.Note, &x.CreatedAt, &x.StartedAt, &x.DoneAt,
		&x.Title, &x.Artist, &x.Duration, &x.Source, &media, &ref); err != nil {
		return nil, err
	}
	x.Ready = media != "" || ref != ""
	return &x, nil
}

// Queue lists a night's entries: singing first, then waiting and asked in
// order, then the ones done (the latest first).
func (s *Store) Queue(ctx context.Context, nightID int64) ([]Entry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+entryCols+` FROM queue q JOIN songs s ON s.id = q.song_id WHERE q.night_id = ?
 ORDER BY CASE q.status WHEN 'singing' THEN 0 WHEN 'waiting' THEN 1 WHEN 'asked' THEN 2 ELSE 3 END, CASE WHEN q.status IN ('done','skipped') THEN q.done_at END DESC, q.position, q.id`, nightID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		x, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// GetEntry returns one entry of a person's nights.
func (s *Store) GetEntry(ctx context.Context, uid, id int64) (*Entry, error) {
	x, err := scanEntry(s.DB.QueryRowContext(ctx, `SELECT `+entryCols+` FROM queue q JOIN songs s ON s.id = q.song_id WHERE q.id = ? AND q.user_id = ?`, id, uid))
	return x, notFound(err)
}

// AddEntry puts a song in a night's queue at the given position (the ones
// after it move down by one).
func (s *Store) AddEntry(ctx context.Context, x *Entry) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE queue SET position = position + 1 WHERE night_id = ? AND status IN ('waiting','asked') AND position >= ?`, x.NightID, x.Position); err != nil {
		return err
	}
	if x.Status == "" {
		x.Status = "waiting"
	}
	now := Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO queue (user_id, night_id, song_id, singer, singer_id, guest_id, position, status, key_shift, note, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		x.UserID, x.NightID, x.SongID, x.Singer, nullI(x.SingerID), nullI(x.GuestID), x.Position, x.Status, x.KeyShift, x.Note, firstNon(x.CreatedAt, now))
	if err != nil {
		return err
	}
	x.ID, _ = res.LastInsertId()
	return tx.Commit()
}

// UpdateEntry saves an entry.
func (s *Store) UpdateEntry(ctx context.Context, x *Entry) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE queue SET song_id=?, singer=?, singer_id=?, guest_id=?, position=?, status=?, key_shift=?, note=?, started_at=?, done_at=? WHERE id = ? AND user_id = ?`,
		x.SongID, x.Singer, nullI(x.SingerID), nullI(x.GuestID), x.Position, x.Status, x.KeyShift, x.Note, x.StartedAt, x.DoneAt, x.ID, x.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteEntry takes an entry out of the queue.
func (s *Store) DeleteEntry(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM queue WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Reorder sets the order of a night's waiting entries (ids not given keep
// their place after the given ones).
func (s *Store) Reorder(ctx context.Context, uid, nightID int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE queue SET position = ? WHERE id = ? AND night_id = ? AND user_id = ?`, i+1, id, nightID, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- performances ----

// Performance is one time a song was sung.
type Performance struct {
	ID            int64    `json:"id"`
	UserID        int64    `json:"-"`
	SongID        *int64   `json:"song_id"`
	NightID       *int64   `json:"night_id"`
	QueueID       *int64   `json:"queue_id"`
	Title         string   `json:"title"`
	Artist        string   `json:"artist"`
	Singer        string   `json:"singer"`
	SingerID      *int64   `json:"singer_id"`
	GuestID       *int64   `json:"guest_id"`
	Practice      bool     `json:"practice"`
	Score         *int     `json:"score"`
	OnTime        *int     `json:"on_time"`
	Steady        *int     `json:"steady"`
	Cheers        int      `json:"cheers"`
	Stars         *float64 `json:"stars"`
	Votes         int      `json:"votes"`
	KeyShift      int      `json:"key_shift"`
	SungMs        int64    `json:"sung_ms"`
	Recording     string   `json:"-"`
	RecordingMime string   `json:"recording_mime"`
	SourceApp     string   `json:"source_app"`
	SungAt        string   `json:"sung_at"`

	HasRecording bool   `json:"has_recording"`
	NightTitle   string `json:"night_title,omitempty"`
}

const perfCols = `p.id, p.user_id, p.song_id, p.night_id, p.queue_id, p.title, p.artist, p.singer, p.singer_id, p.guest_id, p.practice, p.score, p.on_time, p.steady, p.cheers, p.stars, p.votes,
 p.key_shift, p.sung_ms, p.recording, p.recording_mime, p.source_app, p.sung_at, COALESCE(n.title, '')`

func scanPerf(r scanner) (*Performance, error) {
	var x Performance
	var pr int
	if err := r.Scan(&x.ID, &x.UserID, &x.SongID, &x.NightID, &x.QueueID, &x.Title, &x.Artist, &x.Singer, &x.SingerID, &x.GuestID, &pr, &x.Score, &x.OnTime, &x.Steady, &x.Cheers, &x.Stars, &x.Votes,
		&x.KeyShift, &x.SungMs, &x.Recording, &x.RecordingMime, &x.SourceApp, &x.SungAt, &x.NightTitle); err != nil {
		return nil, err
	}
	x.Practice, x.HasRecording = pr == 1, x.Recording != ""
	return &x, nil
}

// PerfFilter narrows the history.
type PerfFilter struct {
	SongID   int64
	SingerID int64
	NightID  int64
	From, To string // YYYY-MM-DD, local days compared on the UTC time's date
	Practice *bool
	Limit    int
}

// ListPerformances returns what was sung, the latest first.
func (s *Store) ListPerformances(ctx context.Context, uid int64, f PerfFilter) ([]Performance, error) {
	where := []string{"p.user_id = ?"}
	args := []any{uid}
	if f.SongID > 0 {
		where = append(where, "p.song_id = ?")
		args = append(args, f.SongID)
	}
	if f.SingerID > 0 {
		where = append(where, "p.singer_id = ?")
		args = append(args, f.SingerID)
	}
	if f.NightID > 0 {
		where = append(where, "p.night_id = ?")
		args = append(args, f.NightID)
	}
	if f.From != "" {
		where = append(where, "p.sung_at >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, "p.sung_at < ?")
		args = append(args, f.To)
	}
	if f.Practice != nil {
		where = append(where, "p.practice = ?")
		args = append(args, boolInt(*f.Practice))
	}
	lim := f.Limit
	if lim <= 0 || lim > 2000 {
		lim = 200
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+perfCols+` FROM performances p LEFT JOIN nights n ON n.id = p.night_id WHERE `+strings.Join(where, " AND ")+` ORDER BY p.sung_at DESC LIMIT ?`, append(args, lim)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Performance{}
	for rows.Next() {
		x, err := scanPerf(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// GetPerformance returns one of a person's performances.
func (s *Store) GetPerformance(ctx context.Context, uid, id int64) (*Performance, error) {
	x, err := scanPerf(s.DB.QueryRowContext(ctx, `SELECT `+perfCols+` FROM performances p LEFT JOIN nights n ON n.id = p.night_id WHERE p.id = ? AND p.user_id = ?`, id, uid))
	return x, notFound(err)
}

// PerformanceByQueue finds the performance kept for a queue entry.
func (s *Store) PerformanceByQueue(ctx context.Context, uid, queueID int64) (*Performance, error) {
	x, err := scanPerf(s.DB.QueryRowContext(ctx, `SELECT `+perfCols+` FROM performances p LEFT JOIN nights n ON n.id = p.night_id WHERE p.queue_id = ? AND p.user_id = ?`, queueID, uid))
	return x, notFound(err)
}

// CreatePerformance keeps a performance and counts the song as sung.
func (s *Store) CreatePerformance(ctx context.Context, x *Performance) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := firstNon(x.SungAt, Now())
	res, err := tx.ExecContext(ctx, `INSERT INTO performances (user_id, song_id, night_id, queue_id, title, artist, singer, singer_id, guest_id, practice, score, on_time, steady, cheers, stars, votes,
 key_shift, sung_ms, recording, recording_mime, source_app, sung_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		x.UserID, nullI(x.SongID), nullI(x.NightID), nullI(x.QueueID), x.Title, x.Artist, x.Singer, nullI(x.SingerID), nullI(x.GuestID), boolInt(x.Practice), x.Score, x.OnTime, x.Steady, x.Cheers, x.Stars, x.Votes,
		x.KeyShift, x.SungMs, x.Recording, x.RecordingMime, x.SourceApp, at)
	if err != nil {
		return err
	}
	x.ID, _ = res.LastInsertId()
	x.SungAt = at
	if x.SongID != nil && *x.SongID > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE songs SET plays = plays + 1, last_sung_at = ? WHERE id = ? AND user_id = ?`, at, *x.SongID, x.UserID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdatePerformance saves the score, the room's votes and the recording.
func (s *Store) UpdatePerformance(ctx context.Context, x *Performance) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE performances SET singer=?, singer_id=?, score=?, on_time=?, steady=?, cheers=?, stars=?, votes=?, sung_ms=?, recording=?, recording_mime=? WHERE id = ? AND user_id = ?`,
		x.Singer, nullI(x.SingerID), x.Score, x.OnTime, x.Steady, x.Cheers, x.Stars, x.Votes, x.SungMs, x.Recording, x.RecordingMime, x.ID, x.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePerformance removes one from the history.
func (s *Store) DeletePerformance(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM performances WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Stat is one row of a "most" list.
type Stat struct {
	ID    int64   `json:"id"`
	Name  string  `json:"name"`
	Extra string  `json:"extra"`
	N     int     `json:"n"`
	Best  *int    `json:"best"`
	Avg   float64 `json:"avg"`
}

// TopSongs are the songs sung most (since a time, "" = ever).
func (s *Store) TopSongs(ctx context.Context, uid int64, since string, n int) ([]Stat, error) {
	return s.stats(ctx, `SELECT COALESCE(song_id, 0), title, artist, COUNT(1), MAX(score), COALESCE(AVG(score), 0) FROM performances WHERE user_id = ? AND sung_at >= ? GROUP BY COALESCE(song_id, title) ORDER BY COUNT(1) DESC, MAX(sung_at) DESC LIMIT ?`, uid, since, n)
}

// TopSingers are the people who sang most.
func (s *Store) TopSingers(ctx context.Context, uid int64, since string, n int) ([]Stat, error) {
	return s.stats(ctx, `SELECT COALESCE(singer_id, 0), singer, '', COUNT(1), MAX(score), COALESCE(AVG(score), 0) FROM performances WHERE user_id = ? AND sung_at >= ? AND singer != '' GROUP BY lower(singer) ORDER BY COUNT(1) DESC LIMIT ?`, uid, since, n)
}

func (s *Store) stats(ctx context.Context, q string, args ...any) ([]Stat, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stat{}
	for rows.Next() {
		var x Stat
		var best sql.NullInt64
		if err := rows.Scan(&x.ID, &x.Name, &x.Extra, &x.N, &best, &x.Avg); err != nil {
			return nil, err
		}
		if best.Valid {
			b := int(best.Int64)
			x.Best = &b
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SungPerDay counts performances per UTC day since a date.
func (s *Store) SungPerDay(ctx context.Context, uid int64, since string) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT substr(sung_at, 1, 10), COUNT(1) FROM performances WHERE user_id = ? AND sung_at >= ? GROUP BY 1`, uid, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var d string
		var n int
		if rows.Scan(&d, &n) == nil {
			out[d] = n
		}
	}
	return out, rows.Err()
}
