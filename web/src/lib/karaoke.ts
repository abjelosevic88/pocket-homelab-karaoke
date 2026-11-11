// Small things many pages share.
import type { LiveSong, ScreenSettings, Song, SongSource } from './types'

export const SOURCE: Record<SongSource, { label: string; icon: string }> = {
  upload: { label: 'Uploaded', icon: 'upload' }, youtube: { label: 'YouTube', icon: 'youtube' }, music: { label: 'Pocket Music', icon: 'music' },
  files: { label: 'Pocket Files', icon: 'archive' }, link: { label: 'From a link', icon: 'link' },
}

export function fmtLen(ms: number): string {
  if (!ms || ms < 0) return ''
  const s = Math.round(ms / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

export function coverOf(s: Pick<Song, 'id' | 'has_cover' | 'cover_url' | 'source'>): string {
  if (s.has_cover || s.source === 'music') return `/api/v1/songs/${s.id}/cover`
  return s.cover_url || ''
}

/** What a song has for the words, in a few words. */
export function wordsOf(s: Song): { text: string; ok: boolean } {
  if (s.has_cdg) return { text: 'CD+G graphics', ok: true }
  if (s.video_words) return { text: 'Words in the video', ok: true }
  if (s.lyrics && s.lyrics_synced) return { text: 'Timed words', ok: true }
  if (s.lyrics) return { text: 'Words, not timed', ok: true }
  return { text: 'No words yet', ok: false }
}

/** A song from the library as the stage plays it on this device (signed in). */
export function stageSong(s: Song): LiveSong {
  return {
    id: s.id, title: s.title, artist: s.artist, language: s.language, kind: s.kind, source: s.source, is_video: s.is_video,
    youtube: !s.has_media && s.source === 'youtube' ? s.source_ref : undefined,
    media: s.has_media || s.source === 'music' ? `/api/v1/songs/${s.id}/media` : undefined, media_mime: s.media_mime,
    cdg: s.has_cdg ? `/api/v1/songs/${s.id}/cdg` : undefined, cover: coverOf(s) || undefined,
    lyrics: s.lyrics, synced: s.lyrics_synced, offset_ms: s.lyrics_offset_ms, video_words: s.video_words, duration_ms: s.duration_ms, voice_cut: s.voice_cut, key_shift: s.key_shift,
  }
}

export function screenSettingsFrom(st: { count_in: boolean; lyrics_size: 'm' | 'l' | 'xl'; lyrics_style: 'sweep' | 'line'; next_line: boolean; translate_to: string; pitch_line: boolean; echo_amount: number }): ScreenSettings {
  return { count_in: st.count_in, lyrics_size: st.lyrics_size, lyrics_style: st.lyrics_style, next_line: st.next_line, translate_to: st.translate_to, pitch_line: st.pitch_line, echo_amount: st.echo_amount, photos: false }
}

export const KEY_LABEL = (k: number) => k === 0 ? 'Original key' : `${k > 0 ? '+' : '−'}${Math.abs(k)}`

export const LANGS: { value: string; label: string }[] = [
  { value: '', label: 'Not set' }, { value: 'en', label: 'English' }, { value: 'sr', label: 'Serbian' }, { value: 'hr', label: 'Croatian' }, { value: 'bs', label: 'Bosnian' },
  { value: 'de', label: 'German' }, { value: 'es', label: 'Spanish' }, { value: 'it', label: 'Italian' }, { value: 'fr', label: 'French' }, { value: 'pt', label: 'Portuguese' },
  { value: 'ru', label: 'Russian' }, { value: 'el', label: 'Greek' }, { value: 'tr', label: 'Turkish' }, { value: 'pl', label: 'Polish' }, { value: 'nl', label: 'Dutch' },
  { value: 'sv', label: 'Swedish' }, { value: 'ja', label: 'Japanese' }, { value: 'ko', label: 'Korean' }, { value: 'zh', label: 'Chinese' }, { value: 'hu', label: 'Hungarian' },
  { value: 'ro', label: 'Romanian' }, { value: 'sl', label: 'Slovenian' }, { value: 'mk', label: 'Macedonian' }, { value: 'bg', label: 'Bulgarian' },
]
export const langName = (v: string) => LANGS.find(l => l.value === v)?.label || v.toUpperCase()

export function score(v: number | null | undefined): string { return v === null || v === undefined ? '—' : String(v) }
