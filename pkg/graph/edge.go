package graph

import (
    "encoding/json"
    "fmt"
)

// Edge — направленная связь между узлами
type Edge struct {
    ID         int64
    Type       string                 // 'contains', 'calls', 'inherits'
    FromID     int64                  // ID узла-источника
    ToID       int64                  // ID узла-цели
    Properties map[string]interface{} // опциональные свойства связи
}

// NewEdge создаёт связь
func NewEdge(id int64, edgeType string, fromID, toID int64) *Edge {
    return &Edge{
        ID:     id,
        Type:   edgeType,
        FromID: fromID,
        ToID:   toID,
    }
}

// Serialize сериализует связь в JSON
func (e *Edge) Serialize() ([]byte, error) {
    data := map[string]interface{}{
        "id":     e.ID,
        "type":   e.Type,
        "from":   e.FromID,
        "to":     e.ToID,
    }
    return json.Marshal(data)
}

// DeserializeEdge восстанавливает связь из байтов
func DeserializeEdge(data []byte) (*Edge, error) {
    var raw map[string]interface{}
    if err := json.Unmarshal(data, &raw); err != nil {
        return nil, err
    }

    id, _ := raw["id"].(float64)
    edgeType, _ := raw["type"].(string)
    fromID, _ := raw["from"].(float64)
    toID, _ := raw["to"].(float64)

    return NewEdge(int64(id), edgeType, int64(fromID), int64(toID)), nil
}

func (e *Edge) String() string {
    return fmt.Sprintf("Edge[%d] -[%s]-> %d", e.FromID, e.Type, e.ToID)
}