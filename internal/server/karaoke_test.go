package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/config"
	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

func noDownloads(c *config.Config) { c.NoDownloads, c.NoLookups = true, true }

// a tiny WAV: what an upload is in the tests
func tinyWAV() []byte {
	wav, _ := synthTune([]tuneNote{tn(60, 1, 48)}, true)
	return wav
}

func (a *app) upload(auth string, files map[string][]byte) (int, map[string]any) {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, data := range files {
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.url+"/api/v1/songs/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, noDownloads)
	code, song := a.upload(a.alice, map[string][]byte{"Traditional - Twinkle Star.wav": tinyWAV(), "Traditional - Twinkle Star.lrc": []byte("[00:01.00]Twinkle twinkle\n[00:03.00]little star")})
	if code != 201 {
		t.Fatalf("upload: %d %v", code, song)
	}
	if song["title"] != "Twinkle Star" || song["artist"] != "Traditional" || song["lyrics_synced"] != true || song["has_media"] != true {
		t.Fatalf("song from files: %v", song)
	}
	sid := id(song)
	night := a.ok(a.alice, "POST", "/api/v1/nights", map[string]any{"title": "Alice's night"})
	nid := id(night)
	singer := a.ok(a.alice, "POST", "/api/v1/singers", map[string]any{"name": "Ana"})
	perf := a.ok(a.alice, "POST", "/api/v1/performances", map[string]any{"song_id": n(sid), "score": 80, "sung_ms": 30000})
	book := a.ok(a.alice, "POST", "/api/v1/books", map[string]any{"name": "Kids", "songs": []int64{n(sid)}})

	// Bob sees none of it, and cannot touch it
	for _, p := range []string{"/api/v1/songs/" + sid, "/api/v1/songs/" + sid + "/media", "/api/v1/nights/" + nid, "/api/v1/singers/" + id(singer), "/api/v1/performances/" + id(perf) + "/recording"} {
		a.expect(404, a.bob, "GET", p, nil)
	}
	a.expect(404, a.bob, "PUT", "/api/v1/songs/"+sid, map[string]any{"title": "mine now"})
	a.expect(404, a.bob, "DELETE", "/api/v1/songs/"+sid, nil)
	a.expect(404, a.bob, "POST", "/api/v1/nights/"+nid+"/control", map[string]any{"action": "start"})
	a.expect(404, a.bob, "PUT", "/api/v1/books/"+id(book)+"/songs/"+sid, nil)
	// Bob cannot queue Alice's song in his own night either
	bn := a.ok(a.bob, "POST", "/api/v1/nights", map[string]any{"title": "Bob's"})
	a.expect(400, a.bob, "POST", "/api/v1/nights/"+id(bn)+"/queue", map[string]any{"song_id": n(sid)})
	if l := a.ok(a.bob, "GET", "/api/v1/songs", nil)["songs"].([]any); len(l) != 0 {
		t.Fatalf("bob sees songs: %v", l)
	}
	if l := a.ok(a.bob, "GET", "/api/v1/nights", nil)["nights"].([]any); len(l) != 1 {
		t.Fatalf("bob sees %d nights", len(l))
	}
	if h := a.ok(a.bob, "GET", "/api/v1/search?q=twinkle", nil)["hits"].([]any); len(h) != 0 {
		t.Fatalf("bob finds alice's songs: %v", h)
	}
	// Alice still has everything
	a.ok(a.alice, "GET", "/api/v1/songs/"+sid, nil)
	if h := a.ok(a.alice, "GET", "/api/v1/search?q=little%20star", nil)["hits"].([]any); len(h) != 1 {
		t.Fatalf("search by words: %v", h)
	}
}

// guest is a phone at a night.
type guest struct {
	a    *app
	code string
	c    *http.Client
}

func (a *app) guest(code string) *guest {
	jar, _ := cookiejar.New(nil)
	return &guest{a: a, code: code, c: &http.Client{Jar: jar}}
}

func (g *guest) call(method, path string, body any) (int, map[string]any) {
	g.a.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, g.a.url+"/api/v1/public/j/"+g.code+path, rd)
	req.Header.Set("Content-Type", "application/json")
	res, err := g.c.Do(req)
	if err != nil {
		g.a.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

func TestNightGuestsQueueAndScreen(t *testing.T) {
	a := newApp(t, noDownloads)
	var songs []string
	for i := 0; i < 4; i++ {
		_, s := a.upload(a.alice, map[string][]byte{fmt.Sprintf("Band - Song %d.wav", i): tinyWAV()})
		songs = append(songs, id(s))
	}
	_, other := a.upload(a.alice, map[string][]byte{"Band - Not queued.wav": tinyWAV()})
	night := a.ok(a.alice, "POST", "/api/v1/nights", map[string]any{"title": "Friday", "per_guest": 2, "rotation": "fair"})
	nid, code, screen := id(night), night["join_code"].(string), night["screen_key"].(string)
	a.ok(a.alice, "POST", "/api/v1/nights/"+nid+"/control", map[string]any{"action": "start"})

	ana, marko := a.guest(strings.ToLower(code)), a.guest(code)
	if c, _ := ana.call("POST", "/requests", map[string]any{"song_id": n(songs[0])}); c != 401 {
		t.Fatalf("a request before joining: %d", c)
	}
	for _, g := range []struct {
		g    *guest
		name string
	}{{ana, "Ana Anić"}, {marko, "Marko"}} {
		if c, m := g.g.call("POST", "", map[string]any{"name": g.name}); c != 201 {
			t.Fatalf("join: %d %v", c, m)
		}
	}
	// Ana asks twice, then Marko once: fair rotation puts Marko before Ana's second
	for _, s := range []string{songs[0], songs[1]} {
		if c, m := ana.call("POST", "/requests", map[string]any{"song_id": n(s)}); c != 201 {
			t.Fatalf("ana asks: %d %v", c, m)
		}
	}
	if c, m := ana.call("POST", "/requests", map[string]any{"song_id": n(songs[2])}); c != 400 || !strings.Contains(fmt.Sprint(m["error"]), "waiting") {
		t.Fatalf("over the limit: %d %v", c, m)
	}
	marko.call("POST", "/requests", map[string]any{"song_id": n(songs[3])})
	_, info := marko.call("GET", "", nil)
	live := info["live"].(map[string]any)
	var order []string
	for _, e := range live["queue"].([]any) {
		order = append(order, e.(map[string]any)["singer"].(string))
	}
	if strings.Join(order, ",") != "Ana,Marko,Ana" {
		t.Fatalf("fair order: %v (guests see first names only)", order)
	}

	// the big screen may play only what is in the queue
	get := func(path string) int {
		res, err := http.Get(a.url + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if c := get("/api/v1/public/screen/" + screen + "/songs/" + songs[0] + "/media"); c != 200 {
		t.Fatalf("screen media: %d", c)
	}
	if c := get("/api/v1/public/screen/" + screen + "/songs/" + id(other) + "/media"); c != 404 {
		t.Fatalf("screen plays a song outside the queue: %d", c)
	}
	if c := get("/api/v1/public/screen/wrong-key-wrong-key-wrong/songs/" + songs[0] + "/media"); c != 404 {
		t.Fatalf("a wrong screen key: %d", c)
	}

	// the song runs: next → started → finished with a score → advance
	report := func(body map[string]any) {
		b, _ := json.Marshal(body)
		res, err := http.Post(a.url+"/api/v1/public/screen/"+screen+"/report", "application/json", bytes.NewReader(b))
		if err != nil || res.StatusCode != 204 {
			t.Fatalf("report %v: %v %v", body, res.StatusCode, err)
		}
		res.Body.Close()
	}
	a.ok(a.alice, "POST", "/api/v1/nights/"+nid+"/control", map[string]any{"action": "next"})
	report(map[string]any{"action": "started"})
	if c, _ := marko.call("POST", "/react", map[string]any{"emoji": "👏"}); c != 204 {
		t.Fatalf("cheer: %d", c)
	}
	st := a.ok(a.alice, "GET", "/api/v1/nights/"+nid, nil)["live"].(map[string]any)
	cur := st["current"].(map[string]any)
	if cur["singer"] != "Ana Anić" || st["phase"] != "singing" {
		t.Fatalf("on the screen: %v %v", cur, st["phase"])
	}
	if c, _ := marko.call("POST", "/vote", map[string]any{"entry_id": num(cur, "id"), "stars": 5}); c != 200 {
		t.Fatalf("vote: %d", c)
	}
	if c, _ := ana.call("POST", "/vote", map[string]any{"entry_id": num(cur, "id"), "stars": 5}); c != 400 {
		t.Fatalf("voting for yourself: %d", c)
	}
	// the screen may not end the night or skip
	b, _ := json.Marshal(map[string]any{"action": "end"})
	res, _ := http.Post(a.url+"/api/v1/public/screen/"+screen+"/report", "application/json", bytes.NewReader(b))
	if res.StatusCode != 403 {
		t.Fatalf("screen ends the night: %d", res.StatusCode)
	}
	res.Body.Close()
	report(map[string]any{"action": "finished", "entry_id": num(cur, "id"), "score": 87, "on_time": 90, "steady": 70, "sung_ms": 40000})
	report(map[string]any{"action": "finished", "entry_id": num(cur, "id"), "score": 10}) // twice: ignored
	perfs := a.ok(a.alice, "GET", "/api/v1/performances?night="+nid, nil)["performances"].([]any)
	if len(perfs) != 1 {
		t.Fatalf("%d performances", len(perfs))
	}
	p := perfs[0].(map[string]any)
	if num(p, "score") != 87 || num(p, "cheers") != 1 || num(p, "stars") != 5 || p["singer"] != "Ana Anić" {
		t.Fatalf("performance: %v", p)
	}
	report(map[string]any{"action": "advance"})
	st = a.ok(a.alice, "GET", "/api/v1/nights/"+nid, nil)["live"].(map[string]any)
	if st["current"].(map[string]any)["singer"] != "Marko" || st["phase"] != "next" {
		t.Fatalf("next singer: %v", st["current"])
	}
	// Ana can ask again now that she has sung one
	if c, m := ana.call("POST", "/requests", map[string]any{"song_id": n(songs[2])}); c != 201 {
		t.Fatalf("ana after singing: %d %v", c, m)
	}
	// a blocked guest cannot ask
	guests := a.ok(a.alice, "GET", "/api/v1/nights/"+nid+"/guests", nil)["guests"].([]any)
	gid := id(guests[1].(map[string]any))
	a.ok(a.alice, "PUT", "/api/v1/nights/"+nid+"/guests/"+gid, map[string]any{"blocked": true})
	if c, _ := marko.call("POST", "/requests", map[string]any{"song_id": n(songs[0])}); c != 403 {
		t.Fatalf("blocked guest asks: %d", c)
	}
	// Bob's guests token does not reach Alice's night through his own
	a.expect(404, a.bob, "GET", "/api/v1/nights/"+nid+"/guests", nil)
	// the end: guests are out
	a.ok(a.alice, "POST", "/api/v1/nights/"+nid+"/control", map[string]any{"action": "end"})
	if c, _ := ana.call("GET", "", nil); c != 404 {
		t.Fatalf("guest after the end: %d", c)
	}
}

func TestApproveAndBook(t *testing.T) {
	a := newApp(t, noDownloads)
	_, s1 := a.upload(a.alice, map[string][]byte{"A - In book.wav": tinyWAV()})
	_, s2 := a.upload(a.alice, map[string][]byte{"A - Not in book.wav": tinyWAV()})
	book := a.ok(a.alice, "POST", "/api/v1/books", map[string]any{"name": "Tonight", "songs": []int64{n(id(s1))}})
	night := a.ok(a.alice, "POST", "/api/v1/nights", map[string]any{"title": "N", "approve": true, "book_id": num(book, "id")})
	g := a.guest(night["join_code"].(string))
	g.call("POST", "", map[string]any{"name": "Sara"})
	_, list := g.call("GET", "/songs", nil)
	if l := list["songs"].([]any); len(l) != 1 {
		t.Fatalf("the song book shows %d songs", len(l))
	}
	if c, _ := g.call("POST", "/requests", map[string]any{"song_id": n(id(s2))}); c != 404 {
		t.Fatalf("a song outside the book: %d", c)
	}
	_, e := g.call("POST", "/requests", map[string]any{"song_id": n(id(s1)), "with": "Nikola"})
	if e["status"] != "asked" || e["singer"] != "Sara & Nikola" {
		t.Fatalf("asked: %v", e)
	}
	nid := id(night)
	st := a.ok(a.alice, "GET", "/api/v1/nights/"+nid, nil)["live"].(map[string]any)
	if len(st["asked"].([]any)) != 1 || len(st["queue"].([]any)) != 0 {
		t.Fatalf("asked/queue: %v", st)
	}
	a.ok(a.alice, "PUT", "/api/v1/nights/"+nid+"/queue/"+id(e), map[string]any{"status": "waiting"})
	st = a.ok(a.alice, "GET", "/api/v1/nights/"+nid, nil)["live"].(map[string]any)
	if len(st["queue"].([]any)) != 1 {
		t.Fatalf("after yes: %v", st["queue"])
	}
}

func TestPlaceFor(t *testing.T) {
	g := func(id int64) *int64 { return &id }
	e := func(guest int64) store.Entry { return store.Entry{GuestID: g(guest)} }
	waiting := []store.Entry{e(1), e(2), e(1), e(3)}
	// rounds: 1:r1, 2:r1, 1:r2, 3:r1 → a new singer (round 1) goes after the last round-1 entry (index 3)
	if at := placeFor("fair", waiting, nil, e(4)); at != 4 {
		t.Fatalf("new singer at %d", at)
	}
	// singer 2's second song (round 2) goes after every round ≤ 2 entry
	if at := placeFor("fair", waiting, nil, e(2)); at != 4 {
		t.Fatalf("second song at %d", at)
	}
	// the one singing now counts as having had their round-1 song
	if at := placeFor("fair", []store.Entry{e(2), e(3)}, &store.Entry{GuestID: g(1)}, e(1)); at != 2 {
		t.Fatalf("singing now at %d", at)
	}
	if at := placeFor("order", waiting, nil, e(4)); at != 4 {
		t.Fatalf("order at %d", at)
	}
}

func TestLinkScopes(t *testing.T) {
	req := func(method, path string) *http.Request { r, _ := http.NewRequest(method, path, nil); return r }
	cases := []struct {
		scopes []string
		method string
		path   string
		want   bool
	}{
		{nil, "GET", "/api/v1/auth/me", true},
		{[]string{"karaoke:read"}, "GET", "/api/v1/songs", true},
		{[]string{"karaoke:read"}, "GET", "/api/v1/day/2026-11-11", true},
		{[]string{"karaoke:read"}, "GET", "/api/v1/songs/3", true},
		{[]string{"karaoke:read"}, "GET", "/api/v1/songs/3/media", false},
		{[]string{"karaoke:read"}, "POST", "/api/v1/songs", false},
		{[]string{"karaoke:write"}, "POST", "/api/v1/songs", true},
		{[]string{"karaoke:write"}, "POST", "/api/v1/nights/2/queue", true},
		{[]string{"karaoke:write"}, "POST", "/api/v1/nights/2/control", false},
		{[]string{"karaoke:write"}, "GET", "/api/v1/export.json", false},
		{[]string{"karaoke:read", "karaoke:write"}, "GET", "/api/v1/settings", false},
		{[]string{"karaoke:read"}, "GET", "/api/v1/backup", false},
	}
	for _, c := range cases {
		if got := linkAllows(c.scopes, req(c.method, c.path)); got != c.want {
			t.Errorf("%v %s %s = %v, want %v", c.scopes, c.method, c.path, got, c.want)
		}
	}
	for _, o := range linkOffers {
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,60}$`).MatchString(o.Scope) {
			t.Errorf("bad scope %q", o.Scope)
		}
	}
}

func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, noDownloads)
	_, s := a.upload(a.alice, map[string][]byte{"Band - One.wav": tinyWAV()})
	a.ok(a.alice, "POST", "/api/v1/songs", map[string]any{"title": "Words only", "lyrics": "la la"})
	a.ok(a.alice, "POST", "/api/v1/singers", map[string]any{"name": "Mila"})
	a.ok(a.alice, "POST", "/api/v1/books", map[string]any{"name": "Book", "songs": []int64{n(id(s))}})
	a.ok(a.alice, "POST", "/api/v1/nights", map[string]any{"title": "A night", "starts_at": "2026-12-01T20:00"})
	a.ok(a.alice, "POST", "/api/v1/performances", map[string]any{"song_id": n(id(s)), "singer": "Mila", "score": 70, "sung_ms": 30000})
	req, _ := http.NewRequest("GET", a.url+"/api/v1/export.json", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var raw map[string]any
	_ = json.Unmarshal(doc, &raw)
	if len(raw["songs"].([]any)) != 2 || len(raw["performances"].([]any)) != 1 {
		t.Fatalf("export: %s", doc)
	}
	importAs := func(tok string) map[string]any {
		req, _ := http.NewRequest("POST", a.url+"/api/v1/import", bytes.NewReader(doc))
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		if res.StatusCode != 200 {
			t.Fatalf("import: %d %v", res.StatusCode, m)
		}
		return m["added"].(map[string]any)
	}
	if added := importAs(a.alice); len(added) != 0 {
		t.Fatalf("importing her own export adds %v", added)
	}
	added := importAs(a.bob)
	if num(added, "songs") != 2 || num(added, "singers") != 1 || num(added, "books") != 1 || num(added, "nights") != 1 || num(added, "performances") != 1 {
		t.Fatalf("bob's first import: %v", added)
	}
	if again := importAs(a.bob); len(again) != 0 {
		t.Fatalf("bob's second import adds %v", again)
	}
	// imported nights get their own codes
	ns := a.ok(a.bob, "GET", "/api/v1/nights", nil)["nights"].([]any)
	if ns[0].(map[string]any)["join_code"] == "" {
		t.Fatal("no join code after import")
	}
}

func TestSettingsArePersonal(t *testing.T) {
	a := newApp(t, noDownloads)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"lyrics_size": "xl", "per_guest": 3, "notify_url": "http://example.test/x"})
	if s := a.ok(a.bob, "GET", "/api/v1/settings", nil); s["lyrics_size"] != "l" || num(s, "per_guest") != 2 || s["notify_url"] != "" {
		t.Fatalf("bob got alice's settings: %v", s)
	}
	if s := a.ok(a.alice, "GET", "/api/v1/settings", nil); s["lyrics_size"] != "xl" || num(s, "per_guest") != 3 {
		t.Fatalf("alice: %v", s)
	}
	// a new night starts from the person's own settings
	nt := a.ok(a.alice, "POST", "/api/v1/nights", map[string]any{"title": "x"})
	if num(nt, "per_guest") != 3 {
		t.Fatalf("night per_guest: %v", nt["per_guest"])
	}
}

func TestLyricsAndYouTube(t *testing.T) {
	lines, synced := parseLyrics("[ti:x]\n[00:01.50]<00:01.50>one <00:02.00>two\n[00:00.50]zero\n[offset:500]\n")
	if !synced || len(lines) != 2 || lines[0].Text != "zero" || lines[1].Text != "one two" || lines[1].T != 1500 {
		t.Fatalf("lrc: %+v", lines)
	}
	if _, s := parseLyrics("just\nwords"); s {
		t.Fatal("plain text is not timed")
	}
	for in, want := range map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10": "dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://youtube.com/shorts/dQw4w9WgXcQ": "dQw4w9WgXcQ", "https://music.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"dQw4w9WgXcQ": "dQw4w9WgXcQ", "https://example.com/watch?v=dQw4w9WgXcQ": "", "not a link": "",
	} {
		if got := youTubeID(in); got != want {
			t.Errorf("youTubeID(%q) = %q, want %q", in, got, want)
		}
	}
	page := `<script>var ytInitialData = {"contents":{"x":[{"videoRenderer":{"videoId":"dQw4w9WgXcQ","title":{"runs":[{"text":"Song (Karaoke Version)"}]},"ownerText":{"runs":[{"text":"Sing King"}]},"lengthText":{"simpleText":"3:32"}}},{"videoRenderer":{"videoId":"aaaaaaaaaaa","title":{"runs":[{"text":"Live"}]}}}]}};</script>`
	vs := parseYouTubePage(page)
	if len(vs) != 1 || vs[0].ID != "dQw4w9WgXcQ" || vs[0].DurationMs != 212000 || !vs[0].Karaoke {
		t.Fatalf("results page: %+v", vs)
	}
	if ti, ar, k := titleFromFile("01 - ABBA - Dancing Queen (Karaoke Version).mp4"); ti != "Dancing Queen" || ar != "ABBA" || !k {
		t.Fatalf("file name: %q %q %v", ti, ar, k)
	}
	for in, want := range map[string][2]string{
		`Twinkle, Twinkle Little Star in the Style of "Traditional" karaoke.x`: {"Twinkle, Twinkle Little Star", "Traditional"},
		"Dancing Queen (in the style of ABBA) Karaoke Version.x":               {"Dancing Queen", "ABBA"},
		"Bohemian Rhapsody - Karaoke.x":                                        {"Bohemian Rhapsody", ""},
	} {
		if ti, ar, _ := titleFromFile(in); ti != want[0] || ar != want[1] {
			t.Errorf("titleFromFile(%q) = %q, %q", in, ti, ar)
		}
	}
	if guessLanguage("Ajde Jano, kolo da igramo, ajde dušo, što je ljubav") != "sr" || guessLanguage("I love you and you love me, the end") != "en" {
		t.Fatal("language guess")
	}
}

func TestDemoData(t *testing.T) {
	a := newApp(t, noDownloads)
	if err := a.s.seedDemo(context.Background(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	songs := a.ok(a.alice, "GET", "/api/v1/songs?limit=100", nil)
	if num(songs, "total") < 15 {
		t.Fatalf("demo songs: %v", songs["total"])
	}
	nights := a.ok(a.alice, "GET", "/api/v1/nights", nil)["nights"].([]any)
	if len(nights) != 6 || nights[0].(map[string]any)["status"] != "live" {
		t.Fatalf("demo nights: %d", len(nights))
	}
	g := a.guest("SING42")
	if c, m := g.call("GET", "", nil); c != 200 || m["title"] != "Friday karaoke" {
		t.Fatalf("demo join: %d %v", c, m)
	}
	// a second run adds nothing
	if err := a.s.seedDemo(context.Background(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	if again := a.ok(a.alice, "GET", "/api/v1/songs?limit=1", nil); num(again, "total") != num(songs, "total") {
		t.Fatal("demo seeded twice")
	}
	if day := a.ok(a.alice, "GET", "/api/v1/day/"+today(), nil); len(day["items"].([]any)) == 0 {
		t.Fatal("no day items today")
	}
}
