package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// seedDemo fills the owner's account with singers, songs (two of them play:
// tunes made right here, the melody in the middle like a lead voice), song
// books, past nights with scores, a live night with guests and a queue, and a
// night planned for the weekend (DEMO_DATA=true): for trying it out, CI and
// the screenshots. Only songs that are free to use have their words here.
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if _, n, _ := s.store.ListSongs(ctx, uid, store.SongFilter{Limit: 1}); n > 0 {
		return nil
	}
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return err
	}
	rnd := rand.New(rand.NewSource(42))
	now := time.Now()

	singerIDs := map[string]int64{}
	for i, g := range []struct{ name, emoji string }{{"Ana", "🌟"}, {"Marko", "🎸"}, {"Jelena", "🦋"}, {"Luka", "🚀"}, {"Mila", "🌈"}} {
		sg := &store.Singer{UserID: uid, Name: g.name, Emoji: g.emoji, Colour: singerColours[i]}
		if err := s.store.SaveSinger(ctx, sg); err != nil {
			return err
		}
		singerIDs[g.name] = sg.ID
	}

	type demoSong struct {
		title, artist, lang, kind, source, ref, tags string
		dur                                          int64
		lyrics                                       string
		videoWords, fav                              bool
		tune                                         []tuneNote
	}
	songs := []demoSong{
		{title: "Twinkle, Twinkle, Little Star", artist: "Traditional", lang: "en", kind: "original", tags: "kids, easy", lyrics: twinkleLRC, fav: true, tune: twinkleTune},
		{title: "Happy Birthday to You", artist: "Traditional", lang: "en", kind: "karaoke", tags: "party, easy", lyrics: birthdayLRC, fav: true, tune: birthdayTune},
		{title: "Amazing Grace", artist: "Traditional", lang: "en", kind: "karaoke", tags: "slow", dur: 245000, lyrics: graceLRC},
		{title: "Auld Lang Syne", artist: "Robert Burns", lang: "en", kind: "karaoke", tags: "new year", dur: 198000, lyrics: auldLRC},
		{title: "Ajde Jano", artist: "Traditional", lang: "sr", kind: "karaoke", tags: "folk", dur: 176000, lyrics: janoLRC, fav: true},
		{title: "Oh! Susanna", artist: "Stephen Foster", lang: "en", kind: "karaoke", tags: "fun", dur: 162000, lyrics: susannaLRC},
		{title: "Scarborough Fair", artist: "Traditional", lang: "en", kind: "karaoke", tags: "slow, duet", dur: 214000, lyrics: scarboroughLRC},
		{title: "Bohemian Rhapsody", artist: "Queen", lang: "en", kind: "karaoke", source: "youtube", ref: "xo1VInw-SKc", dur: 359000, videoWords: true, fav: true, tags: "rock, big finish"},
		{title: "Dancing Queen", artist: "ABBA", lang: "en", kind: "karaoke", source: "youtube", ref: "Gkq7SZfyimI", dur: 231000, videoWords: true, tags: "party"},
		{title: "Let It Go", artist: "Idina Menzel", lang: "en", kind: "karaoke", source: "youtube", ref: "moSFlvxnbgk", dur: 224000, videoWords: true, tags: "kids"},
		{title: "Shallow", artist: "Lady Gaga & Bradley Cooper", lang: "en", kind: "karaoke", source: "youtube", ref: "AmAPRZ8sDuw", dur: 216000, videoWords: true, tags: "duet"},
		{title: "Don't Stop Me Now", artist: "Queen", lang: "en", kind: "karaoke", source: "youtube", ref: "yq0Hn4OJyQ0", dur: 212000, videoWords: true, tags: "rock"},
		{title: "Sweet Caroline", artist: "Neil Diamond", lang: "en", kind: "karaoke", source: "youtube", ref: "bUAsEAc8hRs", dur: 205000, videoWords: true, tags: "everyone sings"},
		{title: "I Will Survive", artist: "Gloria Gaynor", lang: "en", kind: "karaoke", source: "youtube", ref: "ARt9HV9T0w8", dur: 236000, videoWords: true, tags: "party"},
		{title: "Djurdjevdan", artist: "Bijelo Dugme", lang: "sr", kind: "karaoke", source: "youtube", ref: "k0kfYAZS2Vw", dur: 268000, videoWords: true, tags: "ex-yu"},
		{title: "Lipe cvatu", artist: "Bijelo Dugme", lang: "sr", kind: "original", source: "music", ref: "demo-lipe", dur: 254000, tags: "ex-yu"},
		{title: "Zamisli", artist: "Zana", lang: "sr", kind: "original", source: "music", ref: "demo-zamisli", dur: 221000, tags: "ex-yu"},
		{title: "Livin' on a Prayer", artist: "Bon Jovi", lang: "en", kind: "karaoke", source: "upload", dur: 251000, tags: "rock"},
		{title: "Hallelujah", artist: "Leonard Cohen", lang: "en", kind: "karaoke", source: "files", ref: "home:/Music/Karaoke/Hallelujah.mp4", dur: 278000, tags: "slow"},
		{title: "Wonderwall", artist: "Oasis", lang: "en", kind: "karaoke", source: "upload", dur: 258000, tags: "everyone sings"},
	}
	ids := map[string]int64{}
	for i, d := range songs {
		so := &store.Song{UserID: uid, Title: d.title, Artist: d.artist, Language: d.lang, Kind: d.kind, Source: firstNonEmpty(d.source, "upload"), SourceRef: d.ref, Tags: d.tags,
			DurationMs: d.dur, Lyrics: d.lyrics, LyricsSynced: d.lyrics != "", VideoWords: d.videoWords, Favourite: d.fav, IsVideo: d.source == "youtube" || strings.HasSuffix(d.ref, ".mp4"),
			CreatedAt: now.AddDate(0, 0, -120+i*5).UTC().Format(time.RFC3339Nano)}
		if d.lyrics != "" {
			so.LyricsSource = "typed"
			if i > 2 {
				so.LyricsSource = "lrclib"
			}
		}
		if d.source == "youtube" {
			so.CoverURL = ytThumb(d.ref)
		}
		if d.kind == "original" {
			so.VoiceCut = 80
		}
		if len(d.tune) > 0 {
			wav, ms := synthTune(d.tune, d.kind == "original")
			rel, n, err := s.saveMedia(uid, ".wav", bytes.NewReader(wav), 64<<20)
			if err != nil {
				return err
			}
			so.Media, so.MediaSize, so.MediaMime, so.DurationMs = rel, n, "audio/wav", ms
		}
		if err := s.store.CreateSong(ctx, so); err != nil {
			return err
		}
		ids[d.title] = so.ID
	}

	// song books
	for _, b := range []struct {
		name, about string
		songs       []string
	}{
		{"Party starters", "Everyone knows these: good for the first hour.", []string{"Sweet Caroline", "Dancing Queen", "I Will Survive", "Don't Stop Me Now", "Wonderwall", "Happy Birthday to You"}},
		{"Duets", "Two voices, one microphone each.", []string{"Shallow", "Scarborough Fair"}},
		{"Kids", "Easy and short.", []string{"Twinkle, Twinkle, Little Star", "Let It Go", "Happy Birthday to You", "Oh! Susanna"}},
		{"Ex-YU", "Domaće.", []string{"Djurdjevdan", "Lipe cvatu", "Zamisli", "Ajde Jano"}},
	} {
		bk := &store.Book{UserID: uid, Name: b.name, About: b.about}
		if err := s.store.SaveBook(ctx, bk); err != nil {
			return err
		}
		for _, t := range b.songs {
			_ = s.store.SetInBook(ctx, uid, bk.ID, ids[t], true)
		}
	}

	titles := make([]string, 0, len(songs))
	for _, d := range songs {
		titles = append(titles, d.title)
	}
	names := []string{"Ana", "Marko", "Jelena", "Luka", "Mila"}
	sing := func(nightID *int64, at time.Time, who, title string, practice bool) {
		so, _ := s.store.GetSong(ctx, uid, ids[title])
		if so == nil {
			return
		}
		sid := singerIDs[who]
		p := &store.Performance{UserID: uid, SongID: &so.ID, NightID: nightID, Title: so.Title, Artist: so.Artist, Singer: who, SingerID: &sid, Practice: practice,
			SungMs: so.DurationMs, SungAt: at.UTC().Format(time.RFC3339Nano), Cheers: rnd.Intn(18)}
		if !practice || rnd.Intn(3) > 0 {
			base := map[string]int{"Ana": 78, "Marko": 64, "Jelena": 84, "Luka": 58, "Mila": 70}[who]
			sc := clampInt(base+rnd.Intn(21)-10, 20, 99)
			ot, st := clampInt(sc+rnd.Intn(11)-3, 0, 100), clampInt(sc+rnd.Intn(15)-9, 0, 100)
			p.Score, p.OnTime, p.Steady = &sc, &ot, &st
		}
		if nightID != nil {
			stars := 3.2 + float64(rnd.Intn(18))/10
			p.Stars, p.Votes = &stars, 3+rnd.Intn(6)
		}
		_ = s.store.CreatePerformance(ctx, p)
	}

	// four nights gone by
	for k, nt := range []struct {
		title, place string
		ago          int
	}{{"Birthday karaoke", "At home", 75}, {"Friday night songs", "At home", 47}, {"New flat warming", "Marko's", 26}, {"Rainy Saturday", "At home", 12}} {
		start := time.Date(now.Year(), now.Month(), now.Day(), 20, 0, 0, 0, time.Local).AddDate(0, 0, -nt.ago)
		st, en := start.UTC().Format(time.RFC3339Nano), start.Add(3*time.Hour).UTC().Format(time.RFC3339Nano)
		n := &store.Night{UserID: uid, Title: nt.title, Place: nt.place, StartsAt: start.Format("2006-01-02T15:04"), Status: "ended", JoinCode: newJoinCode(), ScreenKey: newToken() + newToken()[:8],
			Rotation: "fair", GuestYouTube: true, PerGuest: 2, Scores: true, Autoplay: 15, StartedAt: &st, EndedAt: &en, CreatedAt: start.AddDate(0, 0, -5).UTC().Format(time.RFC3339Nano)}
		if err := s.store.CreateNight(ctx, n); err != nil {
			return err
		}
		count := 9 + rnd.Intn(8)
		for j := 0; j < count; j++ {
			at := start.Add(time.Duration(j*11+rnd.Intn(4)) * time.Minute)
			who := names[(j+k)%len(names)]
			title := titles[rnd.Intn(len(titles))]
			e := &store.Entry{UserID: uid, NightID: n.ID, SongID: ids[title], Singer: who, Position: j + 1, Status: "done", CreatedAt: at.Add(-20 * time.Minute).UTC().Format(time.RFC3339Nano)}
			sid := singerIDs[who]
			e.SingerID = &sid
			done := at.Add(4 * time.Minute).UTC().Format(time.RFC3339Nano)
			sa := at.UTC().Format(time.RFC3339Nano)
			e.StartedAt, e.DoneAt = &sa, &done
			_ = s.store.AddEntry(ctx, e)
			sing(&n.ID, at, who, title, false)
		}
	}
	// practice at home
	for d := 100; d > 0; d -= 3 + rnd.Intn(5) {
		at := time.Date(now.Year(), now.Month(), now.Day(), 18+rnd.Intn(3), rnd.Intn(60), 0, 0, time.Local).AddDate(0, 0, -d)
		sing(nil, at, names[rnd.Intn(3)], titles[rnd.Intn(7)], true)
	}

	// tonight: live, with guests on their phones and a queue
	tonight := time.Date(now.Year(), now.Month(), now.Day(), 20, 0, 0, 0, time.Local)
	st := now.Add(-50 * time.Minute).UTC().Format(time.RFC3339Nano)
	live := &store.Night{UserID: uid, Title: "Friday karaoke", Place: "Living room", StartsAt: tonight.Format("2006-01-02T15:04"), Status: "live", JoinCode: "SING42", ScreenKey: newToken() + newToken()[:8],
		Rotation: "fair", GuestYouTube: true, PerGuest: 2, Scores: true, Autoplay: 15, StartedAt: &st, About: "Snacks in the kitchen. Pick your songs on your phone!"}
	if err := s.store.CreateNight(ctx, live); err != nil {
		live.JoinCode = newJoinCode()
		if err := s.store.CreateNight(ctx, live); err != nil {
			return err
		}
	}
	guests := map[string]int64{}
	for _, g := range []string{"Sara", "Nikola", "Ivana", "Petar"} {
		gu := &store.Guest{UserID: uid, NightID: live.ID, Name: g}
		if err := s.store.CreateGuest(ctx, gu, hashToken("demo-guest-"+g+randomHex(4))); err != nil {
			return err
		}
		guests[g] = gu.ID
	}
	pos := 0
	for j, done := range []struct{ who, title string }{{"Ana", "Sweet Caroline"}, {"Sara", "Dancing Queen"}, {"Marko", "Don't Stop Me Now"}, {"Nikola", "Djurdjevdan"}} {
		at := now.Add(time.Duration(-45+j*10) * time.Minute)
		pos++
		e := &store.Entry{UserID: uid, NightID: live.ID, SongID: ids[done.title], Singer: done.who, Position: pos, Status: "done"}
		if gid, ok := guests[done.who]; ok {
			e.GuestID = &gid
		} else {
			sid := singerIDs[done.who]
			e.SingerID = &sid
		}
		sa, da := at.UTC().Format(time.RFC3339Nano), at.Add(4*time.Minute).UTC().Format(time.RFC3339Nano)
		e.StartedAt, e.DoneAt = &sa, &da
		_ = s.store.AddEntry(ctx, e)
		if _, ok := guests[done.who]; ok {
			so, _ := s.store.GetSong(ctx, uid, ids[done.title])
			gid := guests[done.who]
			sc := 71 + rnd.Intn(20)
			p := &store.Performance{UserID: uid, SongID: &so.ID, NightID: &live.ID, QueueID: &e.ID, Title: so.Title, Artist: so.Artist, Singer: done.who, GuestID: &gid, Score: &sc, Cheers: 6 + rnd.Intn(10), SungMs: so.DurationMs, SungAt: sa}
			_ = s.store.CreatePerformance(ctx, p)
		} else {
			sing(&live.ID, at, done.who, done.title, false)
		}
	}
	// on the screen now: Jelena is up next
	sa := now.Add(-30 * time.Second).UTC().Format(time.RFC3339Nano)
	cur := &store.Entry{UserID: uid, NightID: live.ID, SongID: ids["Twinkle, Twinkle, Little Star"], Singer: "Jelena", Position: 1, Status: "singing", StartedAt: &sa}
	jid := singerIDs["Jelena"]
	cur.SingerID = &jid
	_ = s.store.AddEntry(ctx, cur)
	for j, w := range []struct{ who, title, status string }{{"Ivana", "Let It Go", "waiting"}, {"Luka", "Livin' on a Prayer", "waiting"}, {"Petar", "I Will Survive", "waiting"},
		{"Sara", "Shallow", "waiting"}, {"Mila", "Happy Birthday to You", "waiting"}, {"Nikola", "Bohemian Rhapsody", "asked"}} {
		e := &store.Entry{UserID: uid, NightID: live.ID, SongID: ids[w.title], Singer: w.who, Position: j + 2, Status: w.status, CreatedAt: now.Add(time.Duration(-30+j*3) * time.Minute).UTC().Format(time.RFC3339Nano)}
		if gid, ok := guests[w.who]; ok {
			e.GuestID = &gid
		} else {
			sid := singerIDs[w.who]
			e.SingerID = &sid
		}
		if w.who == "Sara" {
			e.Singer = "Sara & Nikola"
		}
		_ = s.store.AddEntry(ctx, e)
	}
	phaseAt := now.Add(-5 * time.Second).UTC().Format(time.RFC3339Nano)
	live.CurrentID, live.Phase, live.PhaseAt, live.VoiceNow = &cur.ID, "next", &phaseAt, 80
	_ = s.store.UpdateNight(ctx, live)
	for i := 0; i < 9; i++ {
		s.live.addCheer(cur.ID)
	}

	// the weekend: planned
	sat := now.AddDate(0, 0, (int(time.Saturday)-int(now.Weekday())+7)%7+7)
	planned := &store.Night{UserID: uid, Title: "Mila's birthday", Place: "Garden", StartsAt: time.Date(sat.Year(), sat.Month(), sat.Day(), 19, 30, 0, 0, time.Local).Format("2006-01-02T15:04"),
		Status: "planned", JoinCode: newJoinCode(), ScreenKey: newToken() + newToken()[:8], Rotation: "fair", GuestYouTube: false, Approve: true, PerGuest: 1, Scores: true, Autoplay: 10,
		About: "Kids first until 21:00, then everyone. Bring your own song ideas!"}
	_ = s.store.CreateNight(ctx, planned)

	st2 := s.settingsOf(ctx, uid)
	st2.TranslateTo = ""
	_ = s.store.SaveUserSettings(ctx, uid, st2)
	s.log.Info("demo data ready", "user", u.Email)
	return nil
}

// ---- two tunes made here (the melody in the middle, chords to the sides) ----

type tuneNote struct {
	midi  int     // 0 = rest
	beats float64 // length
	chord int     // root of the chord under it (midi), 0 = keep
}

func tn(m int, b float64, c int) tuneNote { return tuneNote{m, b, c} }

var twinkleTune = []tuneNote{
	tn(0, 4, 48),
	tn(60, 1, 48), tn(60, 1, 0), tn(67, 1, 0), tn(67, 1, 0), tn(69, 1, 53), tn(69, 1, 0), tn(67, 2, 48),
	tn(65, 1, 53), tn(65, 1, 0), tn(64, 1, 48), tn(64, 1, 0), tn(62, 1, 55), tn(62, 1, 0), tn(60, 2, 48),
	tn(67, 1, 48), tn(67, 1, 0), tn(65, 1, 53), tn(65, 1, 0), tn(64, 1, 48), tn(64, 1, 0), tn(62, 2, 55),
	tn(67, 1, 48), tn(67, 1, 0), tn(65, 1, 53), tn(65, 1, 0), tn(64, 1, 48), tn(64, 1, 0), tn(62, 2, 55),
	tn(60, 1, 48), tn(60, 1, 0), tn(67, 1, 0), tn(67, 1, 0), tn(69, 1, 53), tn(69, 1, 0), tn(67, 2, 48),
	tn(65, 1, 53), tn(65, 1, 0), tn(64, 1, 48), tn(64, 1, 0), tn(62, 1, 55), tn(62, 1, 0), tn(60, 2, 48),
	tn(0, 4, 48),
}

// at 100 beats a minute: a beat is 600 ms, the tune starts after 4 beats (2.4 s)
const twinkleLRC = `[ti:Twinkle, Twinkle, Little Star]
[ar:Traditional]
[00:02.40]<00:02.40>Twin<00:03.00>kle, <00:03.60>twin<00:04.20>kle, <00:04.80>lit<00:05.40>tle <00:06.00>star,
[00:07.20]<00:07.20>How <00:07.80>I <00:08.40>won<00:09.00>der <00:09.60>what <00:10.20>you <00:10.80>are!
[00:12.00]<00:12.00>Up <00:12.60>a<00:13.20>bove <00:13.80>the <00:14.40>world <00:15.00>so <00:15.60>high,
[00:16.80]<00:16.80>Like <00:17.40>a <00:18.00>dia<00:18.60>mond <00:19.20>in <00:19.80>the <00:20.40>sky.
[00:21.60]<00:21.60>Twin<00:22.20>kle, <00:22.80>twin<00:23.40>kle, <00:24.00>lit<00:24.60>tle <00:25.20>star,
[00:26.40]<00:26.40>How <00:27.00>I <00:27.60>won<00:28.20>der <00:28.80>what <00:29.40>you <00:30.00>are!`

var birthdayTune = []tuneNote{
	tn(0, 3, 53),
	tn(60, 0.75, 53), tn(60, 0.25, 0), tn(62, 1, 0), tn(60, 1, 0), tn(65, 1, 0), tn(64, 2, 48),
	tn(60, 0.75, 48), tn(60, 0.25, 0), tn(62, 1, 0), tn(60, 1, 0), tn(67, 1, 0), tn(65, 2, 53),
	tn(60, 0.75, 53), tn(60, 0.25, 0), tn(72, 1, 0), tn(69, 1, 0), tn(65, 1, 58), tn(64, 1, 0), tn(62, 1, 0),
	tn(70, 0.75, 58), tn(70, 0.25, 0), tn(69, 1, 53), tn(65, 1, 0), tn(67, 1, 48), tn(65, 2, 53),
	tn(0, 3, 53),
}

// at 100 beats a minute, after 3 beats (1.8 s)
const birthdayLRC = `[ti:Happy Birthday to You]
[ar:Traditional]
[00:01.80]<00:01.80>Hap<00:02.25>py <00:02.40>birth<00:03.00>day <00:03.60>to <00:04.20>you,
[00:05.40]<00:05.40>Hap<00:05.85>py <00:06.00>birth<00:06.60>day <00:07.20>to <00:07.80>you,
[00:09.00]<00:09.00>Hap<00:09.45>py <00:09.60>birth<00:10.20>day <00:10.80>dear <00:11.40>Mi<00:12.00>la,
[00:12.60]<00:12.60>Hap<00:13.05>py <00:13.20>birth<00:13.80>day <00:14.40>to <00:15.00>you!`

const graceLRC = `[00:08.00]Amazing grace, how sweet the sound
[00:17.50]That saved a wretch like me
[00:26.00]I once was lost, but now am found
[00:35.00]Was blind, but now I see
[00:46.00]'Twas grace that taught my heart to fear
[00:55.00]And grace my fears relieved
[01:04.00]How precious did that grace appear
[01:13.00]The hour I first believed`

const auldLRC = `[00:06.00]Should auld acquaintance be forgot
[00:11.00]And never brought to mind?
[00:16.00]Should auld acquaintance be forgot
[00:21.00]And auld lang syne?
[00:27.00]For auld lang syne, my dear
[00:32.00]For auld lang syne
[00:37.00]We'll tak a cup o' kindness yet
[00:42.00]For auld lang syne`

const janoLRC = `[00:05.00]Ajde Jano, kolo da igramo
[00:11.00]Ajde Jano, ajde dušo, kolo da igramo
[00:19.00]Ajde Jano, konja da prodamo
[00:25.00]Ajde Jano, ajde dušo, konja da prodamo
[00:33.00]Ajde Jano, kuću da prodamo
[00:39.00]Ajde Jano, ajde dušo, kuću da prodamo
[00:47.00]Da prodamo, samo da igramo
[00:53.00]Da prodamo, Jano dušo, samo da igramo`

const susannaLRC = `[00:07.00]I come from Alabama with a banjo on my knee
[00:13.00]I'm going to Louisiana, my true love for to see
[00:19.00]It rained all night the day I left, the weather it was dry
[00:25.00]The sun so hot I froze to death, Susanna, don't you cry
[00:31.00]Oh! Susanna, oh don't you cry for me
[00:37.00]For I come from Alabama with a banjo on my knee`

const scarboroughLRC = `[00:10.00]Are you going to Scarborough Fair?
[00:17.00]Parsley, sage, rosemary and thyme
[00:24.00]Remember me to one who lives there
[00:31.00]She once was a true love of mine
[00:40.00]Tell her to make me a cambric shirt
[00:47.00]Parsley, sage, rosemary and thyme
[00:54.00]Without no seams nor needlework
[01:01.00]Then she'll be a true love of mine`

// synthTune writes the tune as a 22 kHz stereo WAV at 100 beats a minute:
// the melody in the middle (where a lead voice sits, so "less voice" takes it
// out) when withVoice, soft chords left and right, a tick on each beat.
func synthTune(tune []tuneNote, withVoice bool) ([]byte, int64) {
	const rate = 22050
	const beat = 0.6
	total := 0.0
	for _, n := range tune {
		total += n.beats * beat
	}
	frames := int(total*rate) + rate/2
	left := make([]float64, frames)
	right := make([]float64, frames)
	freq := func(m int) float64 { return 440 * math.Pow(2, float64(m-69)/12) }
	t0 := 0.0
	chord := 48
	for _, n := range tune {
		if n.chord > 0 {
			chord = n.chord
		}
		start, length := int(t0*rate), int(n.beats*beat*rate)
		for i := 0; i < length && start+i < frames; i++ {
			t := float64(i) / rate
			env := math.Min(1, t*30) * math.Max(0, 1-t/(n.beats*beat)*0.35)
			var c float64
			for k, iv := range []int{0, 4, 7} {
				f := freq(chord + iv)
				c += math.Sin(2*math.Pi*f*t+float64(k)) * 0.06
			}
			c += math.Sin(2*math.Pi*freq(chord-12)*t) * 0.08 // bass, in the middle
			left[start+i] += c*0.9 + math.Sin(2*math.Pi*freq(chord+7)*t)*0.03
			right[start+i] += c*0.9 + math.Sin(2*math.Pi*freq(chord+4)*t)*0.03
			if withVoice && n.midi > 0 {
				f := freq(n.midi)
				v := (math.Sin(2*math.Pi*f*t) + 0.3*math.Sin(4*math.Pi*f*t) + 0.12*math.Sin(6*math.Pi*f*t)) * 0.22 * env * (1 + 0.04*math.Sin(2*math.Pi*5.5*t))
				left[start+i] += v
				right[start+i] += v
			}
		}
		// a soft tick on each beat, to the right
		for b := 0.0; b < n.beats; b++ {
			at := start + int(b*beat*rate)
			for i := 0; i < 400 && at+i < frames; i++ {
				right[at+i] += math.Sin(float64(i)*0.9) * 0.05 * (1 - float64(i)/400)
			}
		}
		t0 += n.beats * beat
	}
	var buf bytes.Buffer
	dataLen := frames * 4
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate*4))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(4))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataLen))
	for i := 0; i < frames; i++ {
		for _, v := range []float64{left[i], right[i]} {
			v = math.Max(-1, math.Min(1, v))
			_ = binary.Write(&buf, binary.LittleEndian, int16(v*30000))
		}
	}
	return buf.Bytes(), int64(float64(frames) / rate * 1000)
}
