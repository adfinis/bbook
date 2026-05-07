package router

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func Router() *mux.Router {
	r := mux.NewRouter()

	r.Use(loggingMiddleware)

	r.Methods("GET").Path("/api/ping").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("pong!"))
	})

	return r
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}
