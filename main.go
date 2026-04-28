package main

import (
	"log"
	"net/http"

	"ai-context/internal/db"
	"ai-context/internal/handler"
)

func main() {
	if err := db.Init(); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}

	http.HandleFunc("/context", handler.HandleContext)
	http.HandleFunc("/session", handler.HandleSession)

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
