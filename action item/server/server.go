package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
)

// Middleware untuk menambahkan CORS dan Cache-Control
func addCorsHeaders(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// Jika request adalah OPTIONS (preflight), balas langsung
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func main() {
	// Path ke folder templates
	templatesPath := filepath.Join("..", "templates")
	fs := http.FileServer(http.Dir(templatesPath))
	http.Handle("/", addCorsHeaders(fs))

	port := "8080"
	fmt.Printf("Starting server at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
