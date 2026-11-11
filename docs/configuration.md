# Configuration

Environment variables; every one has a default. Personal settings (how the words show, the microphone and score, new songs, new nights, notifications, connections, AI) live in the UI per person — the `OPENAI_*`/`NOTIFY_*` values below are copied once into the first admin's settings.

| Variable | Default | What it does |
|---|---|---|
| `BASE_URL` | – | Where the app is (the join link and QR code guests scan, links in pushes, events and other apps, the OIDC callback) |
| `HOST` / `PORT` | `0.0.0.0` / `8080` | Where it listens |
| `DATA_DIR` | `./data` | The database, the song files (`media/<user>/`), yt-dlp's own copy (`bin/`) and the Pocket Core token |
| `DB_PATH` | `$DATA_DIR/pocket-karaoke.db` | The SQLite file |
| `AUTH_MODE` | `local` | `local` (passwords), `proxy` (a header from your reverse proxy), `none` (one user, for trying it out) |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | – | The first account, made on an empty database |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL` / `NOTIFY_FORMAT` | – / `apprise` | Pushes: an Apprise URL, an ntfy topic or a JSON webhook |
| `NOTIFY_HOUR` | `9` | Default hour of the reminder the day before a night |
| `OPENAI_API_KEY` / `OPENAI_MODEL` / `OPENAI_BASE_URL` | – | AI without Pocket Core |
| `CORE_URL` / `CORE_REGISTRATION_KEY` / `CORE_APP_URL` | – | Pocket Core |
| `DOWNLOADS` | `on` | `off`: never keep copies of YouTube songs (no yt-dlp is run; YouTube songs play in YouTube's player) |
| `YTDLP` | – | A yt-dlp of your own; empty: the app keeps its own copy in `DATA_DIR/bin`, updated daily (the image brings one) |
| `LOOKUPS` | `on` | `off`: never ask LRCLIB for words or YouTube for videos (an offline box) |
| `TRUST_PROXY` | `true` | Take the client address from `X-Forwarded-For` (guests' joins, requests and cheers are limited per address) |
| `METRICS_ENABLED` | `true` | Prometheus `/metrics` |
| `MAX_BODY_BYTES` | `67108864` | Largest JSON request (an export being brought back). Song uploads stream to disk, up to 2 GB a file |
| `DEMO_DATA` | `false` | Seed singers, twenty songs (two that really play: tunes made by the app), song books, four past nights with scores, a live night with guests and a queue, and a night planned for the weekend |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | |

Connections to Pocket Music, Files, Photos, Calendar, Chat, Contacts, Translate, Habits and Alerts are made per person under Settings → Connected apps (through Pocket Core with nothing to paste, or directly with a URL and a token).

## What the image brings

`ffmpeg`/`ffprobe` read a song's length, tags and picture; `yt-dlp` (with `python3` and `node`) keeps copies of YouTube songs. A release binary without them still works: songs just come in without their length and picture, and YouTube songs play in YouTube's player.

## Behind a reverse proxy

Uploads can be large (a karaoke video): allow request bodies of a few hundred megabytes, and keep event streams unbuffered (`flush_interval -1` in Caddy) — the big screen, the host's page and the guests' phones follow a night live.
