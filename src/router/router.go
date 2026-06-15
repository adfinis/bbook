package router

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"git.adfinis.com/albertc/bbook/bbook-backend/auth"
	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"git.adfinis.com/albertc/bbook/bbook-backend/server/search"
	"git.adfinis.com/albertc/bbook/bbook-backend/static"
	"git.adfinis.com/albertc/bbook/bbook-backend/templates"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

var tmpl = template.Must(template.ParseFS(templates.FS, "*.tmpl"))

// Contacts are returned one page at a time
const pageSize = 500

type indexTmplData struct {
	Query      string
	Rows       RowsData
	TotalRows  int
	FieldNames []string
}

type integrationsTmplData struct {
	Integrations []integrationView
}

type integrationView struct {
	ID   uuid.UUID
	Name string
	URL  string
}

// RowsData is for the template rows.html.tmpl
type RowsData struct {
	Contacts []database.ContactView
	Spacers  []Spacer
}

// Spacer is a placeholder for a page of contacts that hasn't been fetched yet. Count is the number of rows the eventual page will contain (used to compute its height)
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
		end := min((page + 1) * pageSize, len(contacts))

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
	end := min(start + pageSize, len(contacts))

	return contacts[start:end]
}

func Router() *mux.Router {
	r := mux.NewRouter()

	r.Use(loggingMiddleware)

	r.Methods("GET").Path("/api/ping").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("pong!"))
	})

	// OIDC login flow
	r.Methods("GET").Path("/auth/login").HandlerFunc(auth.LoginHandler)
	r.Methods("GET").Path("/auth/callback").HandlerFunc(auth.CallbackHandler)
	r.Methods("GET").Path("/auth/logout").HandlerFunc(auth.LogoutHandler)

	// Protected routes
	protected := r.NewRoute().Subrouter()
	protected.Use(auth.Middleware)

	protected.Methods("GET").Path("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")

		contacts, err := search.Search(q)
		if err != nil {
			log.Printf("search query %v: %v", q, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			log.Printf("search query %v: %v", q, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			defer cw.Flush()
			cw.Write(database.ContactCSVHeader())
			for _, c := range contacts {
				cw.Write(c.ToCSV())
			}
			return
		}

		// text/vcard
		if strings.Contains(accept, "text/vcard") {
			w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="contacts.vcf"`)
			for _, c := range contacts {
				w.Write([]byte(c.VCard()))
				w.Write([]byte("\r\n"))
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
	protected.Methods("GET").Path("/api/dav").HandlerFunc(handleListIntegrations)
	r.Methods("GET").Path("/api/dav/{id}").HandlerFunc(handleIntegrationExists)
	protected.Methods("POST").Path("/api/dav").HandlerFunc(handleCreateIntegration)
	protected.Methods("PATCH").Path("/api/dav/{id}").HandlerFunc(handleUpdateIntegrationName)
	protected.Methods("DELETE").Path("/api/dav/{id}").HandlerFunc(handleDeleteIntegration)


	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))

	return r
}

func integrationURL(r *http.Request, id uuid.UUID) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/api/dav/%s", scheme, r.Host, id)
}

func renderIntegrations(w http.ResponseWriter, r *http.Request, sub string) {
	rows, err := database.Client.Queries.ListTokensForUser(r.Context(), sub)
	if err != nil {
		log.Printf("list integrations %s: %v", sub, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	views := make([]integrationView, len(rows))
	for i, row := range rows {
		views[i] = integrationView{ID: row.ID, Name: row.Name, URL: integrationURL(r, row.ID)}
	}
	if err := tmpl.ExecuteTemplate(w, "integrations.html.tmpl", integrationsTmplData{Integrations: views}); err != nil {
		log.Printf("rendering integrations: %v", err)
	}
}

func handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	renderIntegrations(w, r, auth.CurrentUserSub(r))
}

func handleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	sub := auth.CurrentUserSub(r)
	if _, err := database.Client.Queries.CreateTokenForUser(r.Context(), sub); err != nil {
		log.Printf("create integration %s: %v", sub, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderIntegrations(w, r, sub)
}

func handleUpdateIntegrationName(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("update integration name %s/%s: %v", sub, id, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("delete integration %s/%s: %v", sub, id, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderIntegrations(w, r, sub)
}

func handleIntegrationExists(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	exists, err := database.Client.Queries.TokenExists(r.Context(), id)
	if err != nil {
		log.Printf("token exists %s: %v", id, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}
	w.Write([]byte("Valid"))
	// w.WriteHeader(http.StatusOK)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}
