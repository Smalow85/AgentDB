package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"ai-context/internal/db"
)

func HandleSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "POST":
		var sess db.Session
		json.NewDecoder(r.Body).Decode(&sess)

		sess.ID = db.Store.NextSessionID()
		sess.CreatedAt = time.Now()

		if err := db.Store.SaveSession(sess); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]int{"id": sess.ID})

	case "GET":
		sessions := db.Store.GetSessions()
		json.NewEncoder(w).Encode(sessions)
	}
}

func HandleContext(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "session_id required"})
		return
	}

	sessID, err := strconv.Atoi(sessionID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid session_id"})
		return
	}

	switch r.Method {
	case "POST":
		var msg db.ContextMsg
		json.NewDecoder(r.Body).Decode(&msg)
		msg.SessionID = sessID
		msg.ID = db.Store.NextContextID(sessID)
		msg.CreatedAt = time.Now()

		if err := db.Store.SaveContext(sessID, msg); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case "GET":
		messages := db.Store.GetContext(sessID)
		json.NewEncoder(w).Encode(messages)

	case "DELETE":
		if err := db.Store.ClearContext(sessID); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
	}
}
