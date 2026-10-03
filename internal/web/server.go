package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"labreserve.local/labreserve/internal/auth"
	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
	_ "time/tzdata"
)

//go:embed templates/*.html static/style.css
var assets embed.FS

type Server struct {
	store        *database.Store
	auth         *auth.Service
	templates    *template.Template
	secureCookie bool
	publicOrigin string
	jakarta      *time.Location
	now          func() time.Time
	handler      http.Handler
}

type PageData struct {
	Title     string
	Error     string
	Login     string
	Identity  *database.Identity
	CSRFToken string
	Resources []database.Resource
	Resource  database.Resource
	Date      string
}

func New(store *database.Store, authentication *auth.Service, cfg config.Config, now func() time.Time) (*Server, error) {
	if now == nil {
		now = time.Now
	}
	templates, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse page templates: %w", err)
	}
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return nil, fmt.Errorf("load Asia/Jakarta timezone: %w", err)
	}
	server := &Server{
		store: store, auth: authentication, templates: templates,
		secureCookie: cfg.CookieSecure, publicOrigin: cfg.PublicOrigin,
		jakarta: jakarta, now: now,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", server.home)
	mux.HandleFunc("GET /login", server.loginPage)
	mux.HandleFunc("POST /login", server.login)
	mux.HandleFunc("POST /logout", server.logout)
	mux.HandleFunc("GET /resources", server.resourceList)
	mux.HandleFunc("GET /resources/{id}", server.resourceDetail)
	mux.HandleFunc("GET /static/style.css", server.stylesheet)
	server.handler = server.middleware(mux)
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, status int, data PageData) {
	if state, ok := requestStateFrom(r); ok && state.hasSession {
		if data.Identity == nil {
			data.Identity = state.session.Identity
		}
		if data.CSRFToken == "" {
			data.CSRFToken = auth.CSRFToken(state.session)
		}
	}
	var body bytes.Buffer
	if err := s.templates.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, "Unable to render page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
