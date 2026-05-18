package router

import (
	"html/template"
	"log"
	"net/http"

	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"git.adfinis.com/albertc/bbook/bbook-backend/templates"
)

var tmpl = template.Must(template.ParseFS(templates.FS, "*.html"))

type indexData struct {
	Contacts []database.Contact
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	contacts, err := database.Client.Queries.AllContacts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "index.html", indexData{Contacts: contacts}); err != nil {
		log.Printf("rendering index: %v", err)
	}
}
