package server

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

const notifyTestBody = "This is how Pocket Karaoke tells you a karaoke night is tomorrow, or that a YouTube song is ready."

func htmlEsc(v string) string { return html.EscapeString(v) }

// push sends a phone message through the person's own notification URL.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, path string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"microphone"}, Link: s.link(path)}); err != nil {
		s.log.Debug("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st := s.settingsOf(ctx, actorID(ctx))
	if st.NotifyURL == "" {
		writeErr(w, http.StatusBadRequest, "add a notification URL first")
		return
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Karaoke", Body: notifyTestBody, Priority: "info", Tags: []string{"microphone"}}); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// nightReminders tells people at their reminder hour about tomorrow's nights (once per night).
func (s *Server) nightReminders(ctx context.Context) {
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	nights, err := s.store.NightsByStatus(ctx, "planned")
	if err != nil {
		return
	}
	for _, n := range nights {
		if !strings.HasPrefix(n.StartsAt, tomorrow) {
			continue
		}
		st := s.settingsOf(ctx, n.UserID)
		if !st.NotifyNight || now.Hour() < st.NotifyHour {
			continue
		}
		key := fmt.Sprintf("remind:%d:%s", n.ID, n.StartsAt)
		if _, _, done := s.store.CacheGet(ctx, key); done {
			continue
		}
		_ = s.store.CachePut(ctx, key, "1")
		when := "tomorrow"
		if len(n.StartsAt) > 11 {
			when += " at " + n.StartsAt[11:16]
		}
		body := fmt.Sprintf("“%s” is %s", n.Title, when)
		if n.Place != "" {
			body += ", " + n.Place
		}
		body += ". Guests join with the code " + n.JoinCode + "."
		if _, nw, _ := s.store.ListSongs(ctx, n.UserID, store.SongFilter{NoWords: true, Limit: 1}); nw > 0 {
			body += fmt.Sprintf(" %d %s still %s no words.", nw, plural(nw, "song", "songs"), plural(nw, "has", "have"))
		}
		path := fmt.Sprintf("/nights/%d", n.ID)
		s.push(ctx, n.UserID, st, "Karaoke tomorrow", body, path)
		s.alert(ctx, n.UserID, "Karaoke tomorrow", body, path, "default")
	}
}

// endForgotten ends nights left live for more than 14 hours (a closed laptop).
func (s *Server) endForgotten(ctx context.Context) {
	nights, err := s.store.NightsByStatus(ctx, "live")
	if err != nil {
		return
	}
	for _, n := range nights {
		last := firstNonEmpty(n.UpdatedAt, n.CreatedAt)
		t, err := time.Parse(time.RFC3339Nano, last)
		if err != nil || time.Since(t) < 14*time.Hour {
			continue
		}
		n := n
		if err := s.control(ctx, &n, controlIn{Action: "end"}, true); err != nil {
			s.log.Debug("end forgotten night", "err", err)
		}
	}
}
