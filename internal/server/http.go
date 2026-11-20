package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-karaoke/internal/store"
)

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Get("/pocket-ask.js", s.core.AskWidget("karaoke"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		// songs playing, uploads, live streams: no time limit
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/nights/{id}/live/stream", s.handleLiveStream)
			r.Get("/songs/{id}/media", s.handleSongMedia)
			r.Get("/songs/{id}/cdg", s.handleSongCDG)
			r.Get("/songs/{id}/cover", s.handleSongCover)
			r.Post("/songs/upload", s.handleUploadSong)
			r.Post("/apps/files/import", s.handleFromFiles)
			r.Get("/performances/{id}/recording", s.handleRecording)
			r.Put("/performances/{id}/recording", s.handleRecording)
			r.Post("/import", s.handleImport)
			r.Get("/export.json", s.handleExportJSON)
			r.Get(strings.TrimPrefix(core.PortablePath, "/api/v1"), portable.Handler())
		})
		r.Get("/public/screen/{key}/stream", s.handleScreenStream)
		r.Get("/public/screen/{key}/songs/{id}/media", s.handleScreenMedia)
		r.Get("/public/screen/{key}/songs/{id}/cdg", s.handleScreenCDG)
		r.Get("/public/screen/{key}/songs/{id}/cover", s.handleScreenCover)
		r.Get("/public/j/{code}/stream", s.handleJoinStream)

		// the big screen and the guests' phones: no account needed
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(60 * time.Second))
			r.Get("/public/screen/{key}", s.handleScreen)
			r.Post("/public/screen/{key}/report", s.handleScreenReport)
			r.Get("/public/screen/{key}/songs/{id}/translation", s.handleScreenTranslation)
			r.Get("/public/screen/{key}/photos", s.handleScreenPhotos)
			r.Get("/public/screen/{key}/photos/{uuid}", s.handleScreenPhoto)
			r.Get("/public/j/{code}", s.handleJoinInfo)
			r.Post("/public/j/{code}", s.handleJoin)
			r.Get("/public/j/{code}/songs", s.handleJoinSongs)
			r.Post("/public/j/{code}/requests", s.handleJoinRequest)
			r.Delete("/public/j/{code}/requests/{eid}", s.handleJoinCancel)
			r.Get("/public/j/{code}/youtube", s.handleJoinYouTube)
			r.Post("/public/j/{code}/youtube", s.handleJoinYouTubeAsk)
			r.Post("/public/j/{code}/react", s.handleJoinReact)
			r.Post("/public/j/{code}/vote", s.handleJoinVote)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(120 * time.Second))
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/setup", s.handleSetup)
			r.Get("/auth/status", s.handleAuthStatus)
			r.Get("/auth/oidc/start", s.handleOIDCStart)
			r.Get("/auth/oidc/callback", s.handleOIDCCallback)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/auth/logout", s.handleLogout)
				r.Get("/auth/me", s.handleMe)
				r.Put("/auth/me", s.handleUpdateMe)

				// for the person, and for the rest of the box: Pocket Day, Search, AI
				r.Get("/home", s.handleHome)
				r.Get("/summary", s.handleSummary)
				r.Get("/day/{date}", s.handleDay)
				r.Get("/search", s.handleSearch)
				r.Get("/stats", s.handleStats)

				// songs
				r.Get("/songs", s.handleSongs)
				r.Post("/songs", s.handleCreateSong)
				r.Get("/songs/{id}", s.handleGetSong)
				r.Put("/songs/{id}", s.handleUpdateSong)
				r.Delete("/songs/{id}", s.handleDeleteSong)
				r.Post("/songs/{id}/cover", s.handleSongCoverUpload)
				r.Post("/songs/{id}/copy", s.handleSongCopy)
				r.Delete("/songs/{id}/copy", s.handleSongCopy)
				r.Get("/songs/{id}/lyrics/lookup", s.handleLyricsLookup)
				r.Get("/songs/{id}/translation", s.handleSongTranslation)
				r.Get("/youtube/search", s.handleYouTubeSearch)
				r.Post("/youtube", s.handleAddYouTube)

				// singers and song books
				r.Get("/singers", s.handleSingers)
				r.Post("/singers", s.handleSaveSinger)
				r.Get("/singers/{id}", s.handleGetSinger)
				r.Put("/singers/{id}", s.handleSaveSinger)
				r.Delete("/singers/{id}", s.handleDeleteSinger)
				r.Get("/books", s.handleBooks)
				r.Post("/books", s.handleSaveBook)
				r.Put("/books/{id}", s.handleSaveBook)
				r.Delete("/books/{id}", s.handleDeleteBook)
				r.Put("/books/{id}/songs/{song}", s.handleBookSong)
				r.Delete("/books/{id}/songs/{song}", s.handleBookSong)

				// nights
				r.Get("/nights", s.handleNights)
				r.Post("/nights", s.handleCreateNight)
				r.Get("/nights/{id}", s.handleGetNight)
				r.Put("/nights/{id}", s.handleUpdateNight)
				r.Delete("/nights/{id}", s.handleDeleteNight)
				r.Post("/nights/{id}/control", s.handleControl)
				r.Post("/nights/{id}/queue", s.handleAddEntry)
				r.Put("/nights/{id}/queue", s.handleReorder)
				r.Put("/nights/{id}/queue/{eid}", s.handleUpdateEntry)
				r.Delete("/nights/{id}/queue/{eid}", s.handleDeleteEntry)
				r.Get("/nights/{id}/guests", s.handleGuests)
				r.Put("/nights/{id}/guests/{gid}", s.handleUpdateGuest)
				r.Post("/nights/{id}/calendar", s.handleNightCalendar)
				r.Post("/nights/{id}/chat", s.handleNightChat)
				r.Post("/nights/{id}/playlist", s.handleNightPlaylist)

				// what was sung
				r.Get("/performances", s.handlePerformances)
				r.Post("/performances", s.handleCreatePerformance)
				r.Put("/performances/{id}", s.handleUpdatePerformance)
				r.Delete("/performances/{id}", s.handleDeletePerformance)
				r.Post("/performances/{id}/files", s.handleRecordingToFiles)

				// the other Pocket apps
				r.Get("/apps", s.handleApps)
				r.Get("/apps/music", s.handleMusicSearch)
				r.Post("/apps/music", s.handleMusicAdd)
				r.Get("/apps/music/cover/{id}", s.handleMusicCover)
				r.Get("/apps/files", s.handleFilesList)
				r.Get("/apps/photos/albums", s.handlePhotoAlbums)
				r.Get("/apps/chats", s.handleChats)
				r.Get("/apps/contacts", s.handleContacts)
				r.Get("/apps/habits", s.handleHabitsList)

				// AI
				r.Get("/ai/status", s.handleAIStatus)
				r.Get("/ai/models", s.handleAIModels)
				r.Post("/ai/tidy", s.handleAITidy)
				r.Post("/ai/suggest", s.handleAISuggest)

				r.Post("/integrations/test", s.handleIntegrationTest)
				r.Post("/notify/test", s.handleNotifyTest)

				r.Get("/users", s.handleListUsers)
				r.Post("/users", s.handleCreateUser)
				r.Put("/users/{id}", s.handleUpdateUser)
				r.Delete("/users/{id}", s.handleDeleteUser)

				r.Get("/settings", s.handleGetSettings)
				r.Put("/settings", s.handlePutSettings)
				r.Get("/settings/system", s.handleSystemInfo)

				r.Get("/tokens", s.handleListTokens)
				r.Post("/tokens", s.handleCreateToken)
				r.Delete("/tokens/{id}", s.handleDeleteToken)

				r.Get("/backup", s.handleBackup)
				r.Get("/export.csv", s.handleExportCSV)
			})
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict), errors.Is(err, core.ErrOperationConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(nil, r.Body, s.cfg.MaxBodyBytes))
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_karaoke_info Build info.\n# TYPE pocket_karaoke_info gauge\npocket_karaoke_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_karaoke_uptime_seconds Process uptime.\n# TYPE pocket_karaoke_uptime_seconds gauge\npocket_karaoke_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_karaoke_http_requests_total HTTP requests served.\n# TYPE pocket_karaoke_http_requests_total counter\npocket_karaoke_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_karaoke_http_errors_total HTTP 5xx responses.\n# TYPE pocket_karaoke_http_errors_total counter\npocket_karaoke_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_karaoke_rows Rows per table.\n# TYPE pocket_karaoke_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_karaoke_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
