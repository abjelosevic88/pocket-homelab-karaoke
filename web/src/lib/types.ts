// The shapes the API sends.

export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = 'core' | 'direct' | ''
export type AppSlug = 'music' | 'files' | 'photos' | 'calendar' | 'chat' | 'contacts' | 'translate' | 'habits' | 'alerts'
type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  count_in: boolean; lyrics_size: 'm' | 'l' | 'xl'; lyrics_style: 'sweep' | 'line'; next_line: boolean; translate_to: string
  scores: boolean; pitch_line: boolean; echo_amount: number; keep_awake: boolean
  lyrics_lookup: boolean; keep_copies: boolean; video_quality: number; default_voice: number; youtube_suffix: string
  rotation: 'fair' | 'order'; approve: boolean; guest_youtube: boolean; per_guest: number; autoplay: number
  photos_album: string; calendar_on_night: boolean; chat_to: string; files_folder: string; habit_id: number; music_playlist: boolean; ai_help: boolean
  notify_night: boolean; notify_request: boolean; notify_copy: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  copies_possible: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'
export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

export type SongSource = 'upload' | 'youtube' | 'music' | 'files' | 'link'

export interface Song {
  id: number; title: string; artist: string; language: string; kind: 'karaoke' | 'original'; source: SongSource; source_ref: string
  media_mime: string; media_size: number; is_video: boolean; cover_url: string; duration_ms: number
  lyrics: string; lyrics_synced: boolean; lyrics_source: string; lyrics_offset_ms: number; video_words: boolean
  key_shift: number; voice_cut: number; tags: string; notes: string; favourite: boolean; plays: number; last_sung_at: string | null
  copy_state: '' | 'waiting' | 'working' | 'ready' | 'failed'; copy_error: string; copy_progress: number; added_by: string; source_app: string
  created_at: string; updated_at: string
  has_media: boolean; has_cdg: boolean; has_cover: boolean; best?: number; books?: number[]
}

export interface Singer { id: number; name: string; colour: string; emoji: string; contact_id: string; notes: string; created_at: string; sung: number; best: number | null; average: number; last_at: string | null }
export interface Book { id: number; name: string; about: string; created_at: string; songs: number }

export type Phase = 'idle' | 'next' | 'singing' | 'paused' | 'score'

export interface Night {
  id: number; title: string; starts_at: string; place: string; about: string; status: 'planned' | 'live' | 'ended'; join_code: string; screen_key: string
  rotation: 'fair' | 'order'; approve: boolean; guest_youtube: boolean; per_guest: number; book_id: number | null; scores: boolean; autoplay: number
  current_id: number | null; phase: Phase; phase_at: string | null; position_ms: number; key_now: number; voice_now: number; volume: number
  calendar_ref: string; started_at: string | null; ended_at: string | null; source_app: string; created_at: string; updated_at: string
  sung: number; waiting: number; guests: number
}

export interface Guest { id: number; night_id: number; name: string; singer_id: number | null; blocked: boolean; joined_at: string; seen_at: string; sung: number; waiting: number }

export interface LiveEntry { id: number; song_id: number; title: string; artist: string; singer: string; guest_id?: number; singer_id?: number; status: 'asked' | 'waiting' | 'singing' | 'done' | 'skipped'; key_shift: number; duration_ms: number; source: SongSource; note?: string }

export interface LiveSong {
  id: number; title: string; artist: string; language: string; kind: string; source: SongSource; youtube?: string; media?: string; media_mime?: string; is_video: boolean
  cdg?: string; cover?: string; lyrics: string; synced: boolean; offset_ms: number; video_words: boolean; duration_ms: number; voice_cut: number; key_shift: number
}

export interface LastResult { entry_id: number; singer: string; title: string; score: number | null; on_time: number | null; steady: number | null; cheers: number; stars: number | null; votes: number; best: boolean }

export interface ScreenSettings { count_in: boolean; lyrics_size: 'm' | 'l' | 'xl'; lyrics_style: 'sweep' | 'line'; next_line: boolean; translate_to: string; pitch_line: boolean; echo_amount: number; photos: boolean }

export interface LiveState {
  type: 'state'
  night_id: number; title: string; status: Night['status']; phase: Phase; phase_at: string; now: string; autoplay: number; position_ms: number
  key_now: number; voice_now: number; volume: number; scores: boolean; join_code: string; join_url: string
  current: LiveEntry | null; song?: LiveSong; next_song?: LiveSong; queue: LiveEntry[]; done?: LiveEntry[]; asked?: LiveEntry[]; result?: LastResult
  screens: number; phones: number; guests: number; cheers: number
  rules: { approve: boolean; youtube: boolean; per_guest: number; book: boolean }
  settings?: ScreenSettings
}

export type LiveMsg = LiveState
  | { type: 'cmd'; cmd: 'play' | 'pause' | 'seek' | 'stop'; value: number; at: string }
  | { type: 'react'; emoji: string; name: string; cheers: number }
  | { type: 'progress'; position_ms: number; playing: boolean }
  | { type: 'gone' }

export interface Performance {
  id: number; song_id: number | null; night_id: number | null; queue_id: number | null; title: string; artist: string; singer: string; singer_id: number | null; guest_id: number | null
  practice: boolean; score: number | null; on_time: number | null; steady: number | null; cheers: number; stars: number | null; votes: number; key_shift: number; sung_ms: number
  recording_mime: string; source_app: string; sung_at: string; has_recording: boolean; night_title?: string
}

export interface Stat { id: number; name: string; extra: string; n: number; best: number | null; avg: number }

export interface Home {
  live: Night | null; next: Night | null; nights: number; favourites: Song[]; recent: Song[]; no_words: Song[]; no_words_total: number
  songs: number; performances: Performance[]; singers: number; copying: Song[]
}

export interface YTVideo { id: string; title: string; channel: string; duration_ms: number; thumb: string; karaoke: boolean; have?: number }
export interface LyricsCandidate { source: string; title: string; artist: string; album: string; duration_ms: number; synced: boolean; lyrics: string; preview: string }
