package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// Pocket AI helps in two places: tidying words pasted from the web into
// singable lines, and suggesting songs from the library for the room. It
// never invents words for a song and never changes anything by itself: the
// person sees the result and keeps it or not.

func (s *Server) aiOf(w http.ResponseWriter, r *http.Request) (*ai.Client, bool) {
	ctx := r.Context()
	st := s.settingsOf(ctx, actorID(ctx))
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "AI help is switched off in Settings → AI")
		return nil, false
	}
	cl, _, why := s.aiFor(ctx, actorID(ctx), st)
	if cl == nil {
		writeErr(w, http.StatusBadRequest, firstNonEmpty(why, "no AI is set up (Settings → AI)"))
		return nil, false
	}
	select {
	case s.aiSem <- struct{}{}:
		return cl, true
	default:
		writeErr(w, http.StatusTooManyRequests, "the AI is busy: try again in a moment")
		return nil, false
	}
}

func (s *Server) aiDone() { <-s.aiSem }

const tidySystem = `You tidy song lyrics that a person pasted from a web page so they can be sung from a karaoke screen.
Rules: keep the words exactly as given — never add, guess or translate words, never write lyrics yourself.
Remove what is not sung: headings like [Chorus] or "Verse 1", chord names (Am, G7, F#m…) on their own lines or above words, tab lines, page junk (ads, "Embed", credits, "You might also like").
Put one sung line per line; keep a blank line between verses. If a chorus is marked "x2" or "Repeat chorus", write it out the times it is sung.
Answer JSON: {"lines": ["…", "", "…"], "language": "two-letter code of the words", "note": "one short sentence on what you changed"}.`

// handleAITidy tidies pasted words into lines (returned, not saved).
func (s *Server) handleAITidy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "paste the words first")
		return
	}
	cl, ok := s.aiOf(w, r)
	if !ok {
		return
	}
	defer s.aiDone()
	var out struct {
		Lines    []string `json:"lines"`
		Language string   `json:"language"`
		Note     string   `json:"note"`
	}
	if err := cl.JSON(r.Context(), tidySystem, clip(in.Text, 30000), &out); err != nil {
		writeErr(w, http.StatusBadGateway, "the AI did not answer: "+err.Error())
		return
	}
	text := strings.TrimSpace(strings.Join(out.Lines, "\n"))
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "language": cleanLang(out.Language), "note": out.Note})
}

const suggestSystem = `You help choose karaoke songs for a group from THEIR OWN song list only (each line: id | title | artist | language | times sung).
Pick songs that fit what the person says about the room (mood, who sings, languages, duets, warming up, the end of the night).
Never name a song that is not in the list. Prefer a mix of well-known songs and ones sung rarely.
Answer JSON: {"picks": [{"id": 12, "why": "a few words"}], "note": "one short sentence"}. At most 12 picks.`

// handleAISuggest suggests songs from the library for the room.
func (s *Server) handleAISuggest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Ask     string `json:"ask"`
		NightID int64  `json:"night_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	uid := actorID(ctx)
	songs, _, err := s.store.ListSongs(ctx, uid, store.SongFilter{Limit: 600, Sort: "plays"})
	if err != nil {
		s.fail(w, err, "songs")
		return
	}
	if len(songs) == 0 {
		writeErr(w, http.StatusBadRequest, "add some songs first: the suggestions come from your own library")
		return
	}
	cl, ok := s.aiOf(w, r)
	if !ok {
		return
	}
	defer s.aiDone()
	var b strings.Builder
	for _, so := range songs {
		fmt.Fprintf(&b, "%d | %s | %s | %s | %d\n", so.ID, so.Title, so.Artist, so.Language, so.Plays)
	}
	ask := firstNonEmpty(strings.TrimSpace(in.Ask), "a good mix for tonight")
	if in.NightID > 0 {
		if n, err := s.store.GetNight(ctx, uid, in.NightID); err == nil {
			q, _ := s.store.Queue(ctx, n.ID)
			var sung []string
			for _, e := range q {
				if e.Status == "done" || e.Status == "singing" || e.Status == "waiting" {
					sung = append(sung, e.Title)
				}
			}
			if len(sung) > 0 {
				ask += "\nAlready sung or waiting tonight (do not pick these): " + strings.Join(sung, "; ")
			}
		}
	}
	var out struct {
		Picks []struct {
			ID  int64  `json:"id"`
			Why string `json:"why"`
		} `json:"picks"`
		Note string `json:"note"`
	}
	if err := cl.JSON(ctx, suggestSystem, "The room: "+clip(ask, 1000)+"\n\nTheir songs:\n"+b.String(), &out); err != nil {
		writeErr(w, http.StatusBadGateway, "the AI did not answer: "+err.Error())
		return
	}
	byID := map[int64]store.Song{}
	for _, so := range songs {
		byID[so.ID] = so
	}
	picks := []map[string]any{}
	for _, p := range out.Picks {
		if so, ok := byID[p.ID]; ok && len(picks) < 12 {
			picks = append(picks, map[string]any{"song": so, "why": clip(p.Why, 200)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"picks": picks, "note": out.Note})
}
