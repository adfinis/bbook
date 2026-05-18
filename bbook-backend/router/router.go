package router

import (
	"html/template"
	"log"
	"net/http"

	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"git.adfinis.com/albertc/bbook/bbook-backend/server/search"
	"git.adfinis.com/albertc/bbook/bbook-backend/static"
	"git.adfinis.com/albertc/bbook/bbook-backend/templates"
	"github.com/gorilla/mux"
)

// Contains all htmx templates
var tmpl = template.Must(template.ParseFS(templates.FS, "*.html"))

type indexTmplData struct {
	Query      string
	Contacts   []database.ContactView
	FieldNames []string
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

		if err := tmpl.ExecuteTemplate(w, "index.html", indexTmplData{
			Query:      q,
			Contacts:   contacts,
			FieldNames: search.FieldNames,
		}); err != nil {
			log.Printf("rendering index: %v", err)
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
