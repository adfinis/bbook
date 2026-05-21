package router

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"git.adfinis.com/albertc/bbook/bbook-backend/server/search"
	"git.adfinis.com/albertc/bbook/bbook-backend/static"
	"git.adfinis.com/albertc/bbook/bbook-backend/templates"
	"github.com/gorilla/mux"
)

var tmpl = template.Must(template.ParseFS(templates.FS, "*.tmpl"))

// Contacts are returned one page at a time
const pageSize = 900

type indexTmplData struct {
	Query      string
	Rows       RowsData
	TotalRows  int
	FieldNames []string
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

	r.Methods("GET").Path("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	r.Methods("GET").Path("/contacts").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")

		contacts, err := search.Search(q)
		if err != nil {
			log.Printf("search query %v: %v", q, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// application/json
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(contacts); err != nil {
				log.Printf("encoding contacts json: %v", err)
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

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))

	return r
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}
