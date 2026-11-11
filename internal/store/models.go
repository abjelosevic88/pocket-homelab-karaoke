package store

import (
	"encoding/json"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Singing.
	CountIn     bool   `json:"count_in"`     // 3-2-1 before the first words
	LyricsSize  string `json:"lyrics_size"`  // m | l | xl
	LyricsStyle string `json:"lyrics_style"` // sweep (the words fill as they are sung) | line (the line lights up)
	NextLine    bool   `json:"next_line"`    // show the line that comes next
	TranslateTo string `json:"translate_to"` // show each line in this language too ('' = off; Pocket Translate)
	Scores      bool   `json:"scores"`       // a score for fun when a microphone listens
	PitchLine   bool   `json:"pitch_line"`   // draw the voice's notes on the screen
	EchoAmount  int    `json:"echo_amount"`  // 0..100 for the microphone through the speakers
	KeepAwake   bool   `json:"keep_awake"`

	// Songs.
	LyricsLookup  bool   `json:"lyrics_lookup"`  // look the words up on LRCLIB when a song comes in
	KeepCopies    bool   `json:"keep_copies"`    // keep a copy of every YouTube song on this server
	VideoQuality  int    `json:"video_quality"`  // highest video height of a copy (480 | 720 | 1080)
	DefaultVoice  int    `json:"default_voice"`  // voice cut for a song with the voice (0..100)
	YouTubeSuffix string `json:"youtube_suffix"` // added to YouTube searches ("karaoke")

	// Nights.
	Rotation        string `json:"rotation"` // fair | order
	Approve         bool   `json:"approve"`
	GuestYouTube    bool   `json:"guest_youtube"`
	PerGuest        int    `json:"per_guest"`
	Autoplay        int    `json:"autoplay"`
	PhotosAlbum     string `json:"photos_album"`      // pictures on the big screen between songs (Pocket Photos)
	CalendarOnNight bool   `json:"calendar_on_night"` // put a planned night in Pocket Calendar
	ChatTo          string `json:"chat_to"`           // the Pocket Chat conversation invitations and results go to
	FilesFolder     string `json:"files_folder"`      // where recordings go in Pocket Files
	HabitID         int64  `json:"habit_id"`          // the Pocket Habits habit practice ticks (0 = none)
	MusicPlaylist   bool   `json:"music_playlist"`    // after a night, make a Pocket Music playlist of what was sung
	AIHelp          bool   `json:"ai_help"`

	// Notifications.
	NotifyNight   bool   `json:"notify_night"`   // a night is tomorrow
	NotifyRequest bool   `json:"notify_request"` // a guest asked for a song that waits for your yes
	NotifyCopy    bool   `json:"notify_copy"`    // a YouTube copy is ready or failed
	NotifyHour    int    `json:"notify_hour"`    // reminders go at this hour
	NotifyURL     string `json:"notify_url"`
	NotifyFormat  string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
