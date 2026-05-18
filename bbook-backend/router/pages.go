package router

import (
	"html/template"
	"log"
	"net/http"

	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"git.adfinis.com/albertc/bbook/bbook-backend/server/search"
	"git.adfinis.com/albertc/bbook/bbook-backend/templates"
)

// Contains all htmx templates
var tmpl = template.Must(template.ParseFS(templates.FS, "*.html"))

type indexTmplData struct {
	Query      string
	Contacts   []database.ContactView
	FieldNames []string
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")

	contacts, err := search.Search(q)
	if err != nil {
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
}
