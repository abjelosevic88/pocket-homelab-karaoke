# Backup

Everything lives in `DATA_DIR`: the SQLite file `pocket-karaoke.db` (WAL mode; every picture is inside it). Take a consistent copy with `pocket-karaoke backup` (writes `DATA_DIR/backups/pocket-karaoke-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin). Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration.

Restore: stop the app, replace `pocket-karaoke.db` (remove the `-wal`/`-shm` files), start.

Every person can also export everything they made — songs with their words and settings, singers, song books, nights and who sang what — as one JSON file (the song files themselves live in `DATA_DIR/media/` and are in the folder backup) (Settings → Backup & data), and bring it back on any box; importing the same file twice adds nothing twice.
