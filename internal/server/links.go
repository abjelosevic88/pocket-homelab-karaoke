package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Karaoke as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "karaoke:read", Name: "See your karaoke", Description: "Read your karaoke songs (titles, artists, the words), your singers, song books and nights — who sang what and the scores — and search them: for your journal, search and Pocket AI."},
	{Scope: "karaoke:write", Name: "Add to your karaoke", Description: "Add songs (a YouTube link, a song file on the web, a title with its words), plan a karaoke night and put a song in a live night's queue for someone (\"put Bohemian Rhapsody on for Ana\"). Changes or removes only what that app added; never starts, ends or deletes your own nights."},
}

// linkRoute is one request a Pocket Core link token may make.
type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(home|summary|search|songs|singers|books|nights|performances|stats|day/\d{4}-\d{2}-\d{2})$`), "karaoke:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(songs|singers|nights)/\d+$`), "karaoke:read"},
	{"GET", regexp.MustCompile(`^/api/v1/songs/\d+/cover$`), "karaoke:read"},
	{"POST", regexp.MustCompile(`^/api/v1/songs$`), "karaoke:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/songs/\d+$`), "karaoke:write"},
	{"POST", regexp.MustCompile(`^/api/v1/nights$`), "karaoke:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/nights/\d+$`), "karaoke:write"},
	{"POST", regexp.MustCompile(`^/api/v1/nights/\d+/queue$`), "karaoke:write"},
	{"GET", regexp.MustCompile(`^/api/v1/youtube/search$`), "karaoke:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "karaoke" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}

// withLinkApp marks a context as acting for another app (tests, and the auth middleware).
func withLinkApp(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, linkAppKey, slug)
}
