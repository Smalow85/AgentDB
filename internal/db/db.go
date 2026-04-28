package db

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type Session struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type ContextMsg struct {
	ID        int       `json:"id"`
	SessionID int       `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Storage struct {
	Sessions map[int]Session      `json:"sessions"`
	Context  map[int][]ContextMsg `json:"context"`
	Mu       sync.RWMutex
}

var Store *Storage

const dataDir = "data"

func Init() error {
	os.MkdirAll(dataDir, 0755)
	Store = &Storage{
		Sessions: make(map[int]Session),
		Context:  make(map[int][]ContextMsg),
	}
	loadSessions()
	loadAllContext()
	return nil
}

func loadSessions() {
	data, _ := os.ReadFile(dataDir + "/sessions.json")
	json.Unmarshal(data, &Store.Sessions)
}

func loadAllContext() {
	entries, _ := os.ReadDir(dataDir)
	for _, e := range entries {
		if e.IsDir() || e.Name() == "sessions.json" {
			continue
		}
		data, _ := os.ReadFile(dataDir + "/" + e.Name())
		var msgs []ContextMsg
		json.Unmarshal(data, &msgs)
		sessID := 0
		fmt.Sscanf(e.Name(), "%d.json", &sessID)
		if sessID > 0 {
			Store.Context[sessID] = msgs
		}
	}
}

func (s *Storage) SaveSession(session Session) error {
	s.Sessions[session.ID] = session
	data, _ := json.MarshalIndent(s.Sessions, "", "  ")
	return os.WriteFile(dataDir+"/sessions.json", data, 0644)
}

func (s *Storage) GetSessions() []Session {
	result := make([]Session, 0, len(s.Sessions))
	for _, v := range s.Sessions {
		result = append(result, v)
	}
	return result
}

func (s *Storage) SaveContext(sessionID int, msg ContextMsg) error {
	if _, ok := s.Context[sessionID]; !ok {
		s.Context[sessionID] = []ContextMsg{}
	}
	s.Context[sessionID] = append(s.Context[sessionID], msg)
	data, _ := json.MarshalIndent(s.Context[sessionID], "", "  ")
	return os.WriteFile(fmt.Sprintf("%s/%d.json", dataDir, sessionID), data, 0644)
}

func (s *Storage) GetContext(sessionID int) []ContextMsg {
	return s.Context[sessionID]
}

func (s *Storage) ClearContext(sessionID int) error {
	s.Context[sessionID] = []ContextMsg{}
	data, _ := json.MarshalIndent(s.Context[sessionID], "", "  ")
	return os.WriteFile(fmt.Sprintf("%s/%d.json", dataDir, sessionID), data, 0644)
}

func (s *Storage) NextSessionID() int {
	maxID := 0
	for k := range s.Sessions {
		if k > maxID {
			maxID = k
		}
	}
	return maxID + 1
}

func (s *Storage) NextContextID(sessionID int) int {
	msgs := s.Context[sessionID]
	if len(msgs) == 0 {
		return 1
	}
	maxID := 0
	for _, m := range msgs {
		if m.ID > maxID {
			maxID = m.ID
		}
	}
	return maxID + 1
}
