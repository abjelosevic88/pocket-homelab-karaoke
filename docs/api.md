# API

Everything is under `/api/v1`, JSON in and out, with a session cookie, `Authorization: Bearer pkar_…` (Settings → API & shortcuts) or a Pocket Core link token. Errors are `{"error": "…"}`. Everything is per person: another person's song, singer, night or performance is a 404.

Times are ISO 8601 UTC; a night's `starts_at` is the local `YYYY-MM-DDTHH:MM` it was typed in. Words are LRC: `[mm:ss.xx]` per line, `<mm:ss.xx>` per word, or plain lines.

## Songs

| | |
|---|---|
| `GET /songs` | `?q=` (title, artist, tags), `source=upload\|youtube\|music\|files\|link`, `language=`, `favourite=1`, `no_words=1`, `book=ID`, `sort=title\|artist\|added\|sung\|plays`, `limit`, `offset` → `{songs, total, languages}` |
| `GET /songs/ID` | `{song, performances, copy_possible}` |
| `POST /songs` | `{url}` — a YouTube link or a song file on the web; or `{title, artist, lyrics}` — just the words |
| `POST /songs/upload` | multipart `file` (several: the song, a `.cdg`, an `.lrc`, a picture — or a `.zip` of them), optional `title`, `artist`, `kind` |
| `PUT /songs/ID` | Any of `title, artist, language, kind (karaoke\|original), lyrics, lyrics_source, lyrics_offset_ms, video_words, key_shift (−6…6), voice_cut (0…100), tags, notes, favourite` |
| `DELETE /songs/ID` | With its files; who sang it stays in the history |
| `GET /songs/ID/media`, `/cdg`, `/cover` | The song (ranges; Pocket Music songs are played through the link), its CD+G graphics, its picture |
| `POST /songs/ID/cover` | multipart `file` (JPEG/PNG/WebP) |
| `POST /songs/ID/copy`, `DELETE /songs/ID/copy` | Keep a copy of a YouTube song on this server (yt-dlp), or drop it |
| `GET /songs/ID/lyrics/lookup` | `?title=&artist=` → `{candidates: [{source: lrclib\|music, synced, lyrics, preview…}]}` |
| `GET /songs/ID/translation` | `?to=sr` → `{lines}` (Pocket Translate; one per line of the words) |
| `GET /youtube/search` | `?q=` (a link works too; the person's "add karaoke" setting is added unless `exact=1`) → `{videos: [{id, title, channel, duration_ms, thumb, karaoke, have}]}` |
| `POST /youtube` | `{id, title, channel, duration_ms}` → the song (the one already there if it was added before) |

## Singers and song books

| | |
|---|---|
| `GET /singers`, `GET /singers/ID` | With how much they sang, their best and average score |
| `POST /singers`, `PUT /singers/ID`, `DELETE /singers/ID` | `{name, emoji, colour, contact_id, notes}` |
| `GET /books`, `POST /books`, `PUT /books/ID`, `DELETE /books/ID` | `{name, about, songs: [IDs]}` |
| `PUT /books/ID/songs/SONG`, `DELETE …` | Put a song in a book, take it out |

## Nights

| | |
|---|---|
| `GET /nights`, `GET /nights/ID` | A night with its live state, guests and what was sung, `join_url`, `screen_url` |
| `POST /nights`, `PUT /nights/ID` | `{title, starts_at, place, about, rotation: fair\|order, approve, guest_youtube, per_guest, book_id, scores, autoplay, calendar}`; `PUT …?new_code=1` a new join code, `?new_screen=1` a new big-screen link |
| `DELETE /nights/ID` | Its queue and guests; what was sung stays |
| `POST /nights/ID/control` | `{action}`: `start`, `end`, `next` (call the next singer), `play`, `pause`, `restart`, `seek` `{value: ms}`, `skip`, `stop` (back to the top of the queue), `key` `{value: −6…6}`, `voice` `{value: 0…100}`, `volume` `{value}` |
| `POST /nights/ID/queue` | `{song_id, singer, singer_id, guest_id, key_shift, note, front}` |
| `PUT /nights/ID/queue` | `{ids: […]}` — the new order of the waiting songs |
| `PUT /nights/ID/queue/EID` | `{singer, key_shift, note, song_id, status: waiting (say yes / put back) \| skipped}` |
| `DELETE /nights/ID/queue/EID` | |
| `GET /nights/ID/guests`, `PUT /nights/ID/guests/GID` | `{name, singer_id, blocked}` |
| `GET /nights/ID/live/stream` | Server-sent events: `state`, `progress`, `react`, `gone` |
| `POST /nights/ID/calendar`, `/chat` `{what: invite\|results, to}`, `/playlist` | Pocket Calendar, Chat, Music |

## Without an account

| | |
|---|---|
| `GET /public/screen/KEY`, `…/stream` | The big screen's state (with the song to play) and its event stream (`state`, `cmd`, `react`) |
| `POST /public/screen/KEY/report` | `{action: started \| progress {value, playing} \| finished {entry_id, score, on_time, steady, sung_ms} \| advance \| next \| play \| pause}` |
| `GET /public/screen/KEY/songs/ID/media\|cdg\|cover\|translation` | Only songs in that night's queue |
| `GET /public/screen/KEY/photos` | Pictures between songs (Pocket Photos) |
| `GET /public/j/CODE`, `POST /public/j/CODE` `{name}` | The night for guests; joining sets a cookie for this night |
| `GET /public/j/CODE/songs` | The song book (`?q=`, `language=`) |
| `POST /public/j/CODE/requests` `{song_id, with, note}`, `DELETE …/requests/EID` | Ask for a song, take your own out |
| `GET /public/j/CODE/youtube?q=`, `POST /public/j/CODE/youtube` | When the host allows it |
| `POST /public/j/CODE/react` `{emoji}`, `POST /public/j/CODE/vote` `{entry_id, stars}` | Cheers and stars |
| `GET /public/j/CODE/stream` | The guests' event stream (first names only) |

## What was sung

| | |
|---|---|
| `GET /performances` | `?song=`, `singer=`, `night=`, `practice=1`, `from=YYYY-MM-DD`, `limit` |
| `POST /performances` | `{song_id, singer, singer_id, practice, score, on_time, steady, key_shift, sung_ms}` (sung alone; practice of more than a minute ticks the Pocket Habits habit) |
| `PUT /performances/ID`, `DELETE /performances/ID` | `{singer, singer_id}` |
| `GET\|PUT /performances/ID/recording` | The recording (PUT the audio body: WebM, Ogg, MP4) |
| `POST /performances/ID/files` | Keep the recording in Pocket Files |
| `GET /stats` | `?period=month\|year` → most sung songs and singers, a year per day, totals |

## For the rest of the box

`GET /home`, `GET /summary` (Homepage), `GET /day/YYYY-MM-DD` (Pocket Day), `GET /search?q=` (songs by title, artist or a line of their words; singers; nights), `GET /export.json`, `GET /export.csv`, `POST /import`, `GET /backup` (admins).

Linked apps (Pocket Core) get `karaoke:read` (the GETs above) and `karaoke:write` (`POST /songs`, `PUT/DELETE /songs/ID` and `/nights/ID` for what that app added, `POST /nights`, `POST /nights/ID/queue`, `GET /youtube/search`). Events: `karaoke.song_added`, `karaoke.night_planned`, `karaoke.night_started`, `karaoke.performed`, `karaoke.night_ended`.
