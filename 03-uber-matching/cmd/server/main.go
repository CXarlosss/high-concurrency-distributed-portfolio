package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	// Serve visual geospatial radar client
	http.Handle("/", http.FileServer(http.Dir("./web")))

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Favicon no-content handler
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	port := ":8082"
	log.Printf("[Server] Uber Matching Engine Geospatial Radar listening on %s", port)
	srv := &http.Server{
		Addr:              port,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
