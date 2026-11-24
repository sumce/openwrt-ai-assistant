// Package server wires the HTTP API, authentication and the embedded
// frontend together.
package server

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/sumce/openwrt-ai-assistant/internal/agent"
	"github.com/sumce/openwrt-ai-assistant/internal/config"
	"github.com/sumce/openwrt-ai-assistant/internal/fakellm"
	"github.com/sumce/openwrt-ai-assistant/internal/safety"
	"github.com/sumce/openwrt-ai-assistant/internal/store"
	"github.com/sumce/openwrt-ai-assistant/web"
)

// Version is stamped at build time via -ldflags "-X ...server.Version=x.y.z".
var Version = "0.1.0"

const sessionCookie = "wrtai_session"

type Config struct {
	Auth     *Auth
	Settings config.Store
	Manager  *agent.Manager
	Backups  *safety.Backups
	Convs    *store.Store
	Dev      bool
}

func New(cfg Config) http.Handler {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()

	// public
	mux.HandleFunc("GET /api/bootstrap", s.handleBootstrap)
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/login", s.handleLogin)

	// authenticated
	mux.HandleFunc("POST /api/logout", s.authed(s.handleLogout))
	mux.HandleFunc("GET /api/settings", s.authed(s.handleGetSettings))
	mux.HandleFunc("PUT /api/settings", s.authed(s.handlePutSettings))
	mux.HandleFunc("POST /api/settings/test", s.authed(s.handleTestProvider))
	mux.HandleFunc("GET /api/conversations", s.authed(s.handleListConvs))
	mux.HandleFunc("DELETE /api/conversations/{id}", s.authed(s.handleDeleteConv))
	mux.HandleFunc("GET /api/conversations/{id}/messages", s.authed(s.handleConvMessages))
	mux.HandleFunc("POST /api/chat", s.authed(s.handleChat))
	mux.HandleFunc("GET /api/runs/active", s.authed(s.handleActiveRuns))
	mux.HandleFunc("GET /api/runs/{id}", s.authed(s.handleRunSnapshot))
	mux.HandleFunc("GET /api/runs/{id}/events", s.authed(s.handleRunEvents))
	mux.HandleFunc("POST /api/runs/{id}/approve", s.authed(s.handleApprove))
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.authed(s.handleCancel))
	mux.HandleFunc("GET /api/backups", s.authed(s.handleListBackups))
	mux.HandleFunc("POST /api/backups/restore", s.authed(s.handleRestoreBackup))
	mux.HandleFunc("POST /api/password", s.authed(s.handleChangePassword))

	// Dev-only scripted LLM endpoint for end-to-end demos without an API key.
	if cfg.Dev {
		mux.Handle("/fake-llm/", http.StripPrefix("/fake-llm", fakellm.Handler()))
	}

	mux.Handle("/", s.staticHandler())
	return mux
}

type Server struct {
	cfg Config
}

// authed wraps handlers with the session cookie check.
func (s *Server) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || !s.cfg.Auth.Check(c.Value) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未登录"})
			return
		}
		next(w, r)
	}
}

// setSession issues the session cookie.
func (s *Server) setSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.cfg.Auth.Token(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
	})
}

func (s *Server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// staticHandler serves the embedded SPA with index.html fallback.
func (s *Server) staticHandler() http.Handler {
	fsys := web.Dist()
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(fsys, p); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
