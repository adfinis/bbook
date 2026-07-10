package router

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"git.adfinis.com/int-infrastructure/bbook/bbook/auth"
	"git.adfinis.com/int-infrastructure/bbook/bbook/carddav"
	"git.adfinis.com/int-infrastructure/bbook/bbook/config"
	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
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
	Integrations []integrationView
}

type integrationView struct {
	ID    uuid.UUID
	Name  string
	Token string
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
		_, _ = w.Write([]byte("pong!"))
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
			log.Printf("search query %s: %v", strconv.Quote(q), err)
			http.Error(w, "search failed", http.StatusInternalServerError)
			return
		}

		if err := tmpl.ExecuteTemplate(w, "index.html.tmpl", indexTmplData{
			Query:      q,
			Rows:       paginateContacts(contacts, q),
			TotalRows:  len(contacts),
			FieldNames: search.FieldNames,
		}); err != nil {
			log.Printf("rendering index: %v", err)
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
			log.Printf("search query %s: %v", strconv.Quote(q), err)
			http.Error(w, "search failed", http.StatusInternalServerError)
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
				log.Printf("encoding contacts json: %v", err)
			}
			return
		}

		// text/csv
		if strings.Contains(accept, "text/csv") {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="contacts.csv"`)
			cw := csv.NewWriter(w)
			_ = cw.Write(database.ContactCSVHeader())
			for _, c := range contacts {
				_ = cw.Write(c.ToCSV())
			}
			cw.Flush()
			if err := cw.Error(); err != nil {
				log.Printf("writing contacts csv: %v", err)
			}
			return
		}

		// text/vcard
		if strings.Contains(accept, "text/vcard") {
			w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="contacts.vcf"`)
			for _, c := range contacts {
				_, _ = w.Write([]byte(c.VCardString())) // #nosec G705 served as text/vcard attachment, not HTML
				_, _ = w.Write([]byte("\r\n"))
			}
			return
		}

		// Client asking for one page in particular
		if pageStr := r.URL.Query().Get("page"); pageStr != "" {
			page, _ := strconv.Atoi(pageStr)
			if err := tmpl.ExecuteTemplate(w, "rows.html.tmpl", RowsData{
				Contacts: pageSlice(contacts, page),
			}); err != nil {
				log.Printf("rendering page %d: %v", page, err)
			}
			return
		}

		// Send first page plus placeholders for the rest
		if err := tmpl.ExecuteTemplate(w, "rows.html.tmpl", paginateContacts(contacts, q)); err != nil {
			log.Printf("rendering rows: %v", err)
		}
	})

	// Mail client integrations

	// Render list of available CardDav integrations
	protected.Methods("GET").Path("/api/dav").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		renderIntegrations(w, r, auth.CurrentUserSub(r))
	})

	// Create a new CardDav token
	protected.Methods("POST").Path("/api/dav").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := auth.CurrentUserSub(r)
		if _, err := database.Client.Queries.CreateTokenForUser(r.Context(), sub); err != nil {
			log.Printf("create integration %s: %v", strconv.Quote(sub), err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderIntegrations(w, r, sub)
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
			log.Printf("update integration name %s/%s: %v", strconv.Quote(sub), strconv.Quote(id.String()), err)
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
			log.Printf("delete integration %s/%s: %v", strconv.Quote(sub), strconv.Quote(id.String()), err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		renderIntegrations(w, r, sub)
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
		id, err := uuid.Parse(strings.TrimSpace(cand))
		if err != nil {
			continue
		}
		if ok, err := database.Client.Queries.TokenValid(ctx, id); err == nil && ok {
			return true
		}
	}
	return false
}

func renderIntegrations(w http.ResponseWriter, r *http.Request, sub string) {
	rows, err := database.Client.Queries.ListTokensForUser(r.Context(), sub)
	if err != nil {
		log.Printf("list integrations %s: %v", strconv.Quote(sub), err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	views := make([]integrationView, len(rows))
	for i, row := range rows {
		views[i] = integrationView{ID: row.ID, Name: row.Name, Token: row.ID.String()}
	}
	if err := tmpl.ExecuteTemplate(w, "integrations.html.tmpl", integrationsTmplData{
		DavURL:       davBaseURL,
		Integrations: views,
	}); err != nil {
		log.Printf("rendering integrations: %v", err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", strconv.Quote(r.RemoteAddr), strconv.Quote(r.Method), strconv.Quote(r.URL.String()))
		next.ServeHTTP(w, r)
	})
}
