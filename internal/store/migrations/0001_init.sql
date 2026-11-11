CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);



-- ---------- Pocket Karaoke ----------
-- Everything belongs to the person who made it (user_id); nobody else on the
-- box sees it, box admins included. Guests at a karaoke night see only what
-- the night's links show: the song list to choose from, the queue (first
-- names only) and the song being sung. Times are UTC ISO strings; a night's
-- starts_at is the local date and time the person typed ("2026-11-14T20:00").
-- Media files (songs, covers, recordings) live in DATA_DIR/media/<user id>/.

CREATE TABLE songs (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title            TEXT NOT NULL,
  artist           TEXT NOT NULL DEFAULT '',
  language         TEXT NOT NULL DEFAULT '',          -- en, sr, …
  kind             TEXT NOT NULL DEFAULT 'karaoke',   -- karaoke (no lead voice) | original (with the voice)
  source           TEXT NOT NULL DEFAULT 'upload',    -- upload | youtube | music | files | link
  source_ref       TEXT NOT NULL DEFAULT '',          -- YouTube video id, Pocket Music song id, Files path, the link
  media            TEXT NOT NULL DEFAULT '',          -- our own copy, relative to the media dir ('' = played from the source)
  media_mime       TEXT NOT NULL DEFAULT '',
  media_size       INTEGER NOT NULL DEFAULT 0,
  is_video         INTEGER NOT NULL DEFAULT 0,
  cdg              TEXT NOT NULL DEFAULT '',          -- CD+G graphics beside an MP3 (classic karaoke discs)
  cover            TEXT NOT NULL DEFAULT '',          -- a picture file, relative to the media dir
  cover_url        TEXT NOT NULL DEFAULT '',          -- or a picture on the web (YouTube's)
  duration_ms      INTEGER NOT NULL DEFAULT 0,
  lyrics           TEXT NOT NULL DEFAULT '',          -- LRC ([mm:ss.xx] per line, <mm:ss.xx> per word) or plain lines
  lyrics_synced    INTEGER NOT NULL DEFAULT 0,
  lyrics_source    TEXT NOT NULL DEFAULT '',          -- typed | file | lrclib | music | tapped | ai
  lyrics_offset_ms INTEGER NOT NULL DEFAULT 0,        -- move all the words earlier (-) or later (+)
  video_words      INTEGER NOT NULL DEFAULT 0,        -- the video shows the words itself
  key_shift        INTEGER NOT NULL DEFAULT 0,        -- the song's usual key, in semitones
  voice_cut        INTEGER NOT NULL DEFAULT 0,        -- 0..100: how much of the lead voice to take out
  tags             TEXT NOT NULL DEFAULT '',
  notes            TEXT NOT NULL DEFAULT '',
  favourite        INTEGER NOT NULL DEFAULT 0,
  plays            INTEGER NOT NULL DEFAULT 0,
  last_sung_at     TEXT,
  copy_state       TEXT NOT NULL DEFAULT '',          -- '' | waiting | working | ready | failed (keeping a copy of a YouTube song)
  copy_error       TEXT NOT NULL DEFAULT '',
  copy_progress    INTEGER NOT NULL DEFAULT 0,
  added_by         TEXT NOT NULL DEFAULT '',          -- a guest's name when a guest asked for it
  source_app       TEXT NOT NULL DEFAULT '',          -- the Pocket app that added it ('' = the person)
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX songs_user ON songs(user_id, title);
CREATE INDEX songs_user_source ON songs(user_id, source, source_ref);

-- Singers are the people who sing often (the household, friends); guests at
-- a night need no singer card.
CREATE TABLE singers (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  colour      TEXT NOT NULL DEFAULT '',
  emoji       TEXT NOT NULL DEFAULT '',
  contact_id  TEXT NOT NULL DEFAULT '',   -- a Pocket Contacts card
  notes       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX singers_user ON singers(user_id, name);

-- Song books: lists of songs (a night's choice, "duets", "kids").
CREATE TABLE books (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  about       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX books_user ON books(user_id);

CREATE TABLE book_songs (
  book_id   INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  song_id   INTEGER NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  position  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (book_id, song_id)
);
CREATE INDEX book_songs_user ON book_songs(user_id, song_id);

-- A karaoke night: planned, live (guests join with the code, the big screen
-- plays the queue) or over.
CREATE TABLE nights (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title         TEXT NOT NULL,
  starts_at     TEXT NOT NULL DEFAULT '',
  place         TEXT NOT NULL DEFAULT '',
  about         TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'planned',   -- planned | live | ended
  join_code     TEXT NOT NULL UNIQUE,              -- what guests type or scan
  screen_key    TEXT NOT NULL UNIQUE,              -- the big screen's link
  rotation      TEXT NOT NULL DEFAULT 'fair',      -- fair (one song each per round) | order (as asked)
  approve       INTEGER NOT NULL DEFAULT 0,        -- the host says yes to each request first
  guest_youtube INTEGER NOT NULL DEFAULT 1,        -- guests may ask for songs from YouTube
  per_guest     INTEGER NOT NULL DEFAULT 2,        -- songs one guest may have waiting
  book_id       INTEGER REFERENCES books(id) ON DELETE SET NULL, -- guests choose from this book only
  scores        INTEGER NOT NULL DEFAULT 1,
  autoplay      INTEGER NOT NULL DEFAULT 15,       -- seconds before the next song starts by itself (0 = wait for a press)
  current_id    INTEGER,                           -- the queue entry on the screen
  phase         TEXT NOT NULL DEFAULT 'idle',      -- idle | next | singing | paused | score
  phase_at      TEXT,
  position_ms   INTEGER NOT NULL DEFAULT 0,
  key_now       INTEGER NOT NULL DEFAULT 0,
  voice_now     INTEGER NOT NULL DEFAULT 0,
  volume        INTEGER NOT NULL DEFAULT 100,
  calendar_ref  TEXT NOT NULL DEFAULT '',
  started_at    TEXT,
  ended_at      TEXT,
  source_app    TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX nights_user ON nights(user_id, starts_at);

-- The people who joined a night from their phone (no account).
CREATE TABLE guests (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  night_id    INTEGER NOT NULL REFERENCES nights(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  token_hash  TEXT NOT NULL UNIQUE,
  singer_id   INTEGER REFERENCES singers(id) ON DELETE SET NULL,
  blocked     INTEGER NOT NULL DEFAULT 0,
  joined_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  seen_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX guests_night ON guests(night_id);
CREATE INDEX guests_user ON guests(user_id);

-- The queue of a night: who sings what, in order.
CREATE TABLE queue (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  night_id    INTEGER NOT NULL REFERENCES nights(id) ON DELETE CASCADE,
  song_id     INTEGER NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
  singer      TEXT NOT NULL DEFAULT '',          -- the name shown ("Ana", "Ana & Marko")
  singer_id   INTEGER REFERENCES singers(id) ON DELETE SET NULL,
  guest_id    INTEGER REFERENCES guests(id) ON DELETE SET NULL,
  position    INTEGER NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'waiting',   -- asked (waits for the host) | waiting | singing | done | skipped
  key_shift   INTEGER NOT NULL DEFAULT 0,
  note        TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  started_at  TEXT,
  done_at     TEXT
);
CREATE INDEX queue_night ON queue(night_id, status, position);
CREATE INDEX queue_user ON queue(user_id);

-- Every time a song was sung (at a night or alone), with the score.
CREATE TABLE performances (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  song_id        INTEGER REFERENCES songs(id) ON DELETE SET NULL,
  night_id       INTEGER REFERENCES nights(id) ON DELETE SET NULL,
  queue_id       INTEGER,
  title          TEXT NOT NULL DEFAULT '',          -- kept when the song goes away
  artist         TEXT NOT NULL DEFAULT '',
  singer         TEXT NOT NULL DEFAULT '',
  singer_id      INTEGER REFERENCES singers(id) ON DELETE SET NULL,
  guest_id       INTEGER REFERENCES guests(id) ON DELETE SET NULL,
  practice       INTEGER NOT NULL DEFAULT 0,        -- sung alone, to practise
  score          INTEGER,                           -- 0..100, NULL = no score (no microphone)
  on_time        INTEGER,                           -- % of the words' time a voice was heard
  steady         INTEGER,                           -- % of the singing on a held note
  cheers         INTEGER NOT NULL DEFAULT 0,        -- reactions from the room
  stars          REAL,                              -- what the room voted (1..5)
  votes          INTEGER NOT NULL DEFAULT 0,
  key_shift      INTEGER NOT NULL DEFAULT 0,
  sung_ms        INTEGER NOT NULL DEFAULT 0,
  recording      TEXT NOT NULL DEFAULT '',          -- relative to the media dir
  recording_mime TEXT NOT NULL DEFAULT '',
  source_app     TEXT NOT NULL DEFAULT '',
  sung_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX performances_user ON performances(user_id, sung_at);
CREATE INDEX performances_song ON performances(user_id, song_id);
