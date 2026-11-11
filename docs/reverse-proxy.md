# Reverse proxy

Set `BASE_URL` to the public address and `SECURE_COOKIES=true` behind HTTPS. See [deploy/reverse-proxy/Caddyfile](../deploy/reverse-proxy/Caddyfile).

- **Uploads**: a song comes in one request (a karaoke video can be a few hundred MB). If your proxy limits request bodies, allow at least 1 GB.
- **Live pages**: the big screen, the host's page and the guests' phones keep event streams open (`/api/v1/nights/ID/live/stream`, `/api/v1/public/screen/KEY/stream`, `/api/v1/public/j/CODE/stream`). Don't buffer them (`flush_interval -1` in Caddy).
- **Pages without an account**: the big screen (`/screen/<key>`, a long secret key per night) and the guests' page (`/j/<code>`, a six-letter code; only while the night is not over). Joining, asking for songs and cheering are limited per address — keep `TRUST_PROXY=true` so that is the visitor's address. Guests in your house reach the app on your network; put it on a public address only if guests outside it should join.
- **YouTube**: the big screen plays YouTube songs in YouTube's own player (`youtube-nocookie.com`); a strict `Content-Security-Policy` must allow it as a frame.
