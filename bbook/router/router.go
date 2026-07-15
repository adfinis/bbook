package router

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.adfinis.com/int-infrastructure/bbook/bbook/auth"
	"git.adfinis.com/int-infrastructure/bbook/bbook/carddav"
	"git.adfinis.com/int-infrastructure/bbook/bbook/config"
	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"git.adfinis.com/int-infrastructure/bbook/bbook/logging"
	"git.adfinis.com/int-infrastructure/bbook/bbook/server/search"
	"git.adfinis.com/int-infrastructure/bbook/bbook/static"
	"git.adfinis.com/int-infrastructure/bbook/bbook/templates"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

var tmpl = template.Must(template.ParseFS(templates.FS, "*.tmpl"))

// Contacts are returned one page at a time.
const pageSize = 500

type indexTmplData struct {
	Query      string
	Rows       RowsData
	TotalRows  int
	FieldNames []string
}

type integrationsTmplData struct {
	DavURL       string
	New          newIntegration
	Integrations []integrationView
}

// just-created integration, rendered with its one-time token.
type newIntegration struct {
	ID    string
	Token string
}

type integrationView struct {
	ID   uuid.UUID
	Name string
}

// RowsData is for the template rows.html.tmpl.
type RowsData struct {
	Contacts []database.ContactView
	Spacers  []Spacer
}

// Spacer is a placeholder for a page of contacts that hasn't been fetched yet. Count is the number of rows the eventual page will contain (used to compute its height).
type Spacer struct {
	Query string
	Page  int
	Count int
}

// Returns the first page of contacts, plus a Spacer for each remaining page.
func paginateContacts(contacts []database.ContactView, q string) RowsData {
	firstEnd := min(pageSize, len(contacts))

	var spacers []Spacer
	for page := 1; page*pageSize < len(contacts); page++ {
		end := min((page+1)*pageSize, len(contacts))

		spacers = append(spacers, Spacer{
			Query: q,
			Page:  page,
			Count: end - page*pageSize,
		})
	}
	return RowsData{
		Contacts: contacts[:firstEnd],
		Spacers:  spacers,
	}
}

// Contacts that belong to the given page.
func pageSlice(contacts []database.ContactView, page int) []database.ContactView {
	page = max(page, 0)

	start := page * pageSize
	if start > len(contacts) {
		return nil
	}
	end := min(start+pageSize, len(contacts))

	return contacts[start:end]
}

// davBaseURL is the CardDAV server URL clients point at, shared across tokens.
var davBaseURL string

func Router(cfg *config.Config) *mux.Router {
	davBaseURL = strings.TrimSuffix(cfg.BaseURL, "/") + carddav.Prefix + "/"
	r := mux.NewRouter()

	r.Use(loggingMiddleware)

	r.Methods("GET").Path("/api/ping").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("pong!")) //nolint:errcheck
	})

	// OIDC login flow
	r.Methods("GET").Path("/auth/login").HandlerFunc(auth.LoginHandler)
	r.Methods("GET").Path("/auth/callback").HandlerFunc(auth.CallbackHandler)
	r.Methods("GET").Path("/auth/logout").HandlerFunc(auth.LogoutHandler)

	// Protected routes (OIDC login required)
	protected := r.NewRoute().Subrouter()
	protected.Use(auth.Middleware)

	protected.Methods("GET").Path("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")

		contacts, err := search.Search(q)
		if err != nil {
			slog.ErrorContext(r.Context(), "search failed", "query", q, "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := tmpl.ExecuteTemplate(w, "index.html.tmpl", indexTmplData{
			Query:      q,
			Rows:       paginateContacts(contacts, q),
			TotalRows:  len(contacts),
			FieldNames: search.FieldNames,
		}); err != nil {
			slog.ErrorContext(r.Context(), "rendering index", "err", err)
		}
	})

	// Fetches contacts given a query q.
	// If Accept: application/json
	//   Dumps the raw JSON
	//
	// ?page=N
	//   HTML rows for the specified page
	//
	// otherwise
	//   HTML rows for the first page, plus placeholders for subsequent pages
	protected.Methods("GET").Path("/contacts").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")

		contacts, err := search.Search(q)
		if err != nil {
			slog.ErrorContext(r.Context(), "search failed", "query", q, "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Format of contacts
		accept := r.Header.Get("Accept")
		switch r.URL.Query().Get("format") {
		case "json":
			accept = "application/json"
		case "csv":
			accept = "text/csv"
		case "vcf":
			accept = "text/vcard"
		}

		// application/json
		if strings.Contains(accept, "application/json") {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(contacts); err != nil {
				slog.ErrorContext(r.Context(), "encoding contacts json", "err", err)
			}
			return
		}

		// text/csv
		if strings.Contains(accept, "text/csv") {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="contacts.csv"`)
			cw := csv.NewWriter(w)
			cw.Write(database.ContactCSVHeader()) //nolint:errcheck
			for _, c := range contacts {
				cw.Write(c.ToCSV()) //nolint:errcheck
			}
			cw.Flush()
			if err := cw.Error(); err != nil {
				slog.ErrorContext(r.Context(), "writing contacts csv", "err", err)
			}
			return
		}

		// text/vcard
		if strings.Contains(accept, "text/vcard") {
			w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="contacts.vcf"`)
			for _, c := range contacts {
				w.Write([]byte(c.VCardString())) //nolint:errcheck,gosec // G705: served as a text/vcard attachment, not HTML
				w.Write([]byte("\r\n"))          //nolint:errcheck
			}
			return
		}

		// Client asking for one page in particular
		if pageStr := r.URL.Query().Get("page"); pageStr != "" {
			page, err := strconv.Atoi(pageStr)
			if err != nil {
				http.Error(w, "invalid page", http.StatusBadRequest)
				return
			}
			if err := tmpl.ExecuteTemplate(w, "rows.html.tmpl", RowsData{
				Contacts: pageSlice(contacts, page),
			}); err != nil {
				slog.ErrorContext(r.Context(), "rendering page", "page", page, "err", err)
			}
			return
		}

		// Send first page plus placeholders for the rest
		if err := tmpl.ExecuteTemplate(w, "rows.html.tmpl", paginateContacts(contacts, q)); err != nil {
			slog.ErrorContext(r.Context(), "rendering rows", "err", err)
		}
	})

	// Mail client integrations

	// Render list of available CardDav integrations
	protected.Methods("GET").Path("/api/dav").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		renderIntegrations(w, r, auth.CurrentUserSub(r), newIntegration{})
	})

	// Create a new CardDav token.
	protected.Methods("POST").Path("/api/dav").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := auth.CurrentUserSub(r)
		secret, hash, err := newDavToken()
		if err != nil {
			slog.ErrorContext(r.Context(), "create integration: generating token", "sub", sub, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		row, err := database.Client.Queries.CreateTokenForUser(r.Context(), database.CreateTokenForUserParams{
			UserSub:   sub,
			TokenHash: hash,
		})
		if err != nil {
			slog.ErrorContext(r.Context(), "create integration", "sub", sub, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderIntegrations(w, r, sub, newIntegration{ID: row.ID.String(), Token: secret})
	})

	// Update a CardDav integration
	protected.Methods("PATCH").Path("/api/dav/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := auth.CurrentUserSub(r)
		id, err := uuid.Parse(mux.Vars(r)["id"])
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.PostFormValue("name"))
		if _, err := database.Client.Queries.UpdateTokenName(r.Context(), database.UpdateTokenNameParams{
			ID:      id,
			UserSub: sub,
			Name:    name,
		}); err != nil {
			slog.ErrorContext(r.Context(), "update integration name", "sub", sub, "id", id, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete a CardDav integration
	protected.Methods("DELETE").Path("/api/dav/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := auth.CurrentUserSub(r)
		id, err := uuid.Parse(mux.Vars(r)["id"])
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if _, err := database.Client.Queries.DeleteTokenForUser(r.Context(), database.DeleteTokenForUserParams{
			ID:      id,
			UserSub: sub,
		}); err != nil {
			slog.ErrorContext(r.Context(), "delete integration", "sub", sub, "id", id, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderIntegrations(w, r, sub, newIntegration{})
	})

	// CardDAV for mail clients

	davProtected := r.PathPrefix(carddav.Prefix).Subrouter()

	// Ensure the request is authenticated with a CardDav integration token
	davProtected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, pass, ok := r.BasicAuth()
			if !ok || !validDavToken(r.Context(), pass) {
				w.Header().Set("WWW-Authenticate", `Basic realm="bbook CardDAV"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	davSrv := carddav.Handler()
	davProtected.Methods("OPTIONS", "PROPFIND", "REPORT").Handler(davSrv)
	davProtected.PathPrefix("/principal/").Handler(davSrv)

	r.Handle("/.well-known/carddav", davSrv)

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))

	return r
}

func validDavToken(ctx context.Context, candidates ...string) bool {
	for _, cand := range candidates {
		h := sha256.Sum256([]byte(strings.TrimSpace(cand)))
		if ok, err := database.Client.Queries.TokenValidByHash(ctx, h[:]); err == nil && ok {
			return true
		}
	}
	return false
}

func newDavToken() (secret string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	secret = "bbook_" + base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(secret))
	return secret, h[:], nil
}

func renderIntegrations(w http.ResponseWriter, r *http.Request, sub string, created newIntegration) {
	rows, err := database.Client.Queries.ListTokensForUser(r.Context(), sub)
	if err != nil {
		slog.ErrorContext(r.Context(), "list integrations", "sub", sub, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	views := make([]integrationView, len(rows))
	for i, row := range rows {
		views[i] = integrationView{ID: row.ID, Name: row.Name}
	}
	if err := tmpl.ExecuteTemplate(w, "integrations.html.tmpl", integrationsTmplData{
		DavURL:       davBaseURL,
		New:          created,
		Integrations: views,
	}); err != nil {
		slog.ErrorContext(r.Context(), "rendering integrations", "err", err)
	}
}

// captures the response status for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := logging.ContextWith(r.Context(), slog.String("req_id", requestID()))
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))
		slog.InfoContext(ctx, "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"dur_ms", time.Since(start).Milliseconds(),
			"ip", clientIP(r),
		)
	})
}

// short random identifier used to correlate a request's logs.
func requestID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
