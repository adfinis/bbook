package router

import (
	"log"
	"net/http"

	"git.adfinis.com/albertc/bbook/bbook-backend/static"
	"github.com/gorilla/mux"
)

func Router() *mux.Router {
	r := mux.NewRouter()

	r.Use(loggingMiddleware)

	r.Methods("GET").Path("/api/ping").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("pong!"))
	})

	r.Methods("GET").Path("/").HandlerFunc(handleIndex)

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))

	return r
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}
