package shortener

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

// reserved paths can't be used as aliases because they are routes.
var reserved = map[string]bool{"api": true, "healthz": true}

type createRequest struct {
	URL        string `json:"url"`
	Alias      string `json:"alias"`
	TTLSeconds int64  `json:"ttlSeconds"`
}

type linkResponse struct {
	*Link
	ShortURL string `json:"shortUrl"`
}

// Server wires the store to HTTP routes.
type Server struct {
	Store   *Store
	BaseURL string
}

// Routes returns the HTTP handler, using Go 1.22+ method-aware routing.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("POST /api/links", s.create)
	mux.HandleFunc("GET /api/links/{code}", s.stats)
	mux.HandleFunc("DELETE /api/links/{code}", s.remove)
	mux.HandleFunc("GET /{code}", s.redirect)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})
	return logRequests(mux)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "body must be JSON like {\"url\": \"https://…\"}")
		return
	}
	if !validURL(req.URL) {
		writeErr(w, http.StatusUnprocessableEntity, "url must be an absolute http(s) URL")
		return
	}
	if req.Alias != "" && (!aliasPattern.MatchString(req.Alias) || reserved[strings.ToLower(req.Alias)]) {
		writeErr(w, http.StatusUnprocessableEntity, "alias must be 3-32 letters, digits, - or _")
		return
	}
	l, err := s.Store.Create(req.URL, req.Alias, time.Duration(req.TTLSeconds)*time.Second)
	if errors.Is(err, ErrTaken) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save link")
		return
	}
	writeJSON(w, http.StatusCreated, s.response(l))
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	l, err := s.Store.Get(r.PathValue("code"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.response(l))
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Delete(r.PathValue("code")); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	l, err := s.Store.Resolve(r.PathValue("code"))
	switch {
	case errors.Is(err, ErrExpired):
		http.Error(w, "this link has expired", http.StatusGone)
	case err != nil:
		http.NotFound(w, r)
	default:
		http.Redirect(w, r, l.URL, http.StatusFound)
	}
}

func (s *Server) response(l *Link) linkResponse {
	return linkResponse{Link: l, ShortURL: strings.TrimRight(s.BaseURL, "/") + "/" + l.Code}
}

func validURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Microsecond))
	})
}

const indexHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Go URL shortener</title><style>body{font:16px system-ui;max-width:560px;margin:60px auto;padding:0 16px}input,button{font:inherit;padding:10px;width:100%;margin:6px 0;box-sizing:border-box}#out{margin-top:12px;word-break:break-all}</style></head>
<body><h1>Shorten a link</h1><form id="f"><input id="u" type="url" required placeholder="https://example.com/very/long/path"><input id="a" placeholder="custom alias (optional)"><button>Shorten</button></form><p id="out"></p>
<script>f.onsubmit=async e=>{e.preventDefault();const r=await fetch('/api/links',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({url:u.value,alias:a.value})});const j=await r.json();out.innerHTML=r.ok?'<a href="'+j.shortUrl+'">'+j.shortUrl+'</a>':j.error}</script></body></html>`
