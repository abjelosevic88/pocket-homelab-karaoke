package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// The words of a song are kept as LRC: "[01:02.30]a line" per line, and
// optionally "<01:02.80>word" for the time of each word (the "enhanced" LRC
// karaoke files use). Words without times are plain lines. The browser
// (web/src/lib/lyrics.ts) reads the same format.

var (
	reLrcTime = regexp.MustCompile(`\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	reLrcWord = regexp.MustCompile(`<(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?>`)
	reLrcTag  = regexp.MustCompile(`^\[(ti|ar|al|by|offset|length|re|ve|au|#):([^\]]*)\]\s*$`)
)

// lyricLine is one line of words with its time (-1 = not timed).
type lyricLine struct {
	T    int64  `json:"t"`
	Text string `json:"text"`
}

func lrcMs(m, s, frac string) int64 {
	mi, _ := strconv.Atoi(m)
	se, _ := strconv.Atoi(s)
	ms := 0
	if frac != "" {
		f := frac
		for len(f) < 3 {
			f += "0"
		}
		ms, _ = strconv.Atoi(f[:3])
	}
	return int64(mi*60000 + se*1000 + ms)
}

// parseLyrics reads LRC or plain text into lines (word marks dropped), sorted by time.
func parseLyrics(text string) ([]lyricLine, bool) {
	var out []lyricLine
	synced := false
	offset := int64(0)
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		if m := reLrcTag.FindStringSubmatch(line); m != nil {
			if m[1] == "offset" {
				offset, _ = strconv.ParseInt(strings.TrimSpace(m[2]), 10, 64)
			}
			continue
		}
		times := reLrcTime.FindAllStringSubmatch(line, -1)
		words := strings.TrimSpace(reLrcWord.ReplaceAllString(reLrcTime.ReplaceAllString(line, ""), ""))
		words = strings.Join(strings.Fields(words), " ")
		if len(times) == 0 {
			if words == "" && len(out) == 0 {
				continue
			}
			out = append(out, lyricLine{T: -1, Text: words})
			continue
		}
		synced = true
		for _, t := range times {
			out = append(out, lyricLine{T: max(0, lrcMs(t[1], t[2], t[3])-offset), Text: words})
		}
	}
	if synced {
		var timed []lyricLine
		for _, l := range out {
			if l.T >= 0 {
				timed = append(timed, l)
			}
		}
		sort.SliceStable(timed, func(i, j int) bool { return timed[i].T < timed[j].T })
		out = timed
	}
	for len(out) > 0 && out[len(out)-1].Text == "" {
		out = out[:len(out)-1]
	}
	return out, synced
}

func isSynced(text string) bool {
	_, s := parseLyrics(text)
	return s
}

// lyricsPlain is the words without times (search, AI, translations).
func lyricsPlain(text string) string {
	lines, _ := parseLyrics(text)
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// lrcTags reads the title and artist of an LRC file.
func lrcTags(text string) (title, artist string) {
	for _, raw := range strings.Split(text, "\n") {
		if m := reLrcTag.FindStringSubmatch(strings.TrimSpace(raw)); m != nil {
			switch m[1] {
			case "ti":
				title = strings.TrimSpace(m[2])
			case "ar":
				artist = strings.TrimSpace(m[2])
			}
		}
	}
	return
}

// cleanLyrics trims what came in: no BOM, \n line ends, at most 64 KB.
func cleanLyrics(v string) string {
	v = strings.TrimPrefix(v, "\ufeff")
	v = strings.ReplaceAll(v, "\r\n", "\n")
	v = strings.ReplaceAll(v, "\r", "\n")
	return strings.TrimSpace(clip(v, 64<<10))
}

func lrcStamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("[%02d:%02d.%02d]", ms/60000, (ms/1000)%60, (ms%1000)/10)
}

// toLRC writes timed lines as LRC.
func toLRC(lines []lyricLine) string {
	var b strings.Builder
	for _, l := range lines {
		if l.T >= 0 {
			b.WriteString(lrcStamp(l.T))
		}
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// ---- LRCLIB: a free, open library of timed words (lrclib.net) ----

type lrclibHit struct {
	ID           int64   `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

// lyricsCandidate is one set of words to choose from.
type lyricsCandidate struct {
	Source   string `json:"source"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int64  `json:"duration_ms"`
	Synced   bool   `json:"synced"`
	Lyrics   string `json:"lyrics"`
	Preview  string `json:"preview"`
	Score    int    `json:"-"`
}

func (s *Server) lrclibSearch(ctx context.Context, title, artist string) ([]lyricsCandidate, error) {
	title, artist = cleanForLookup(title), cleanForLookup(artist)
	if s.cfg.NoLookups {
		return nil, &userErr{"looking words up is switched off on this box (LOOKUPS=off)"}
	}
	if title == "" {
		return nil, &userErr{"the song needs a title to look its words up"}
	}
	q := url.Values{"track_name": {title}}
	if artist != "" {
		q.Set("artist_name", artist)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://lrclib.net/api/search?"+q.Encode(), nil)
	req.Header.Set("User-Agent", s.cfg.UserAgent+" (Pocket Karaoke; https://github.com/abjelosevic88/pocket-homelab-karaoke)")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LRCLIB did not answer: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LRCLIB answered HTTP %d", res.StatusCode)
	}
	var hits []lrclibHit
	if err := json.NewDecoder(res.Body).Decode(&hits); err != nil {
		return nil, err
	}
	out := []lyricsCandidate{}
	for _, h := range hits {
		if h.Instrumental || (h.PlainLyrics == "" && h.SyncedLyrics == "") {
			continue
		}
		c := lyricsCandidate{Source: "lrclib", Title: h.TrackName, Artist: h.ArtistName, Album: h.AlbumName, Duration: int64(h.Duration * 1000)}
		if h.SyncedLyrics != "" {
			c.Lyrics, c.Synced = h.SyncedLyrics, true
		} else {
			c.Lyrics = h.PlainLyrics
		}
		c.Preview = clip(strings.Join(firstLines(lyricsPlain(c.Lyrics), 3), " / "), 160)
		c.Score = matchScore(title, artist, c.Title, c.Artist)
		if c.Synced {
			c.Score += 3
		}
		out = append(out, c)
		if len(out) >= 12 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

var reLookupNoise = regexp.MustCompile(`(?i)\s*[\(\[][^\)\]]*(karaoke|instrumental|backing|lyrics?|official|video|audio|version|hd|hq|remaster)[^\)\]]*[\)\]]|\s+-\s+(karaoke|instrumental).*$|\bkaraoke\b`)

func cleanForLookup(v string) string {
	return strings.Join(strings.Fields(reLookupNoise.ReplaceAllString(v, "")), " ")
}

func matchScore(title, artist, t2, a2 string) int {
	n := func(v string) string { return strings.ToLower(strings.Join(strings.Fields(v), " ")) }
	sc := 0
	if n(title) == n(t2) {
		sc += 10
	} else if strings.Contains(n(t2), n(title)) || strings.Contains(n(title), n(t2)) {
		sc += 5
	}
	if artist != "" {
		if n(artist) == n(a2) {
			sc += 8
		} else if strings.Contains(n(a2), n(artist)) || strings.Contains(n(artist), n(a2)) {
			sc += 4
		}
	}
	return sc
}

// findLyrics looks the words of a song up (LRCLIB, and Pocket Music when the
// song is from there) and keeps the best timed set. Called when a song comes
// in and the person allows looking up.
func (s *Server) findLyrics(ctx context.Context, uid int64, so *store.Song) bool {
	if so.Lyrics != "" || so.VideoWords || so.CDG != "" {
		return false
	}
	var best *lyricsCandidate
	if so.Source == "music" {
		if c, err := s.musicLyrics(ctx, uid, so.SourceRef); err == nil && c != nil {
			best = c
		}
	}
	if best == nil || !best.Synced {
		if list, err := s.lrclibSearch(ctx, so.Title, so.Artist); err == nil {
			for i := range list {
				c := list[i]
				if c.Score < 10 {
					continue
				}
				if so.DurationMs > 0 && c.Duration > 0 && absI64(so.DurationMs-c.Duration) > 15000 && c.Synced {
					continue // a different recording: the times would not fit
				}
				if best == nil || (c.Synced && !best.Synced) {
					best = &c
				}
				break
			}
		}
	}
	if best == nil {
		return false
	}
	cur, err := s.store.GetSong(ctx, uid, so.ID)
	if err != nil || cur.Lyrics != "" {
		return false
	}
	cur.Lyrics, cur.LyricsSynced, cur.LyricsSource = cleanLyrics(best.Lyrics), best.Synced, best.Source
	if cur.Language == "" {
		cur.Language = guessLanguage(lyricsPlain(cur.Lyrics))
	}
	if s.store.UpdateSong(ctx, cur) == nil {
		*so = *cur
		return true
	}
	return false
}

func absI64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// guessLanguage tells a few languages apart by their letters and small words
// (enough to group a library; Pocket Translate knows better).
func guessLanguage(text string) string {
	low := " " + strings.ToLower(text) + " "
	if strings.ContainsAny(low, "ђћџљњ") {
		return "sr"
	}
	if strings.ContainsAny(low, "абвгдежзиклмнопрстуфхцчшщыэюя") {
		if strings.ContainsAny(low, "ыэъ") {
			return "ru"
		}
		return "sr"
	}
	count := func(words ...string) int {
		n := 0
		for _, w := range words {
			n += strings.Count(low, " "+w+" ")
		}
		return n
	}
	scores := map[string]int{
		"en": count("the", "and", "you", "i", "me", "my", "is", "love", "your", "it"),
		"sr": count("i", "je", "da", "se", "ne", "ti", "mi", "sam", "što", "šta", "kad", "ljubav") + 3*strings.Count(low, "č") + 3*strings.Count(low, "ć") + 3*strings.Count(low, "đ") + 2*strings.Count(low, "ž") + 2*strings.Count(low, "š"),
		"de": count("und", "ich", "du", "die", "der", "das", "nicht", "ist"),
		"es": count("el", "la", "que", "y", "mi", "tu", "de", "amor", "con"),
		"it": count("il", "che", "non", "di", "per", "sono", "amore", "mi"),
		"fr": count("le", "la", "je", "tu", "et", "les", "est", "pas", "mon"),
	}
	best, bn := "", 3
	for k, v := range scores {
		if v > bn {
			best, bn = k, v
		}
	}
	return best
}
