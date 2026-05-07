package graph

import (
    "encoding/json"
    "fmt"
)

// Node — узел графа
type Node struct {
    ID         int64
    Labels     []string
    Properties map[string]interface{}
}

// NewNode создаёт узел
func NewNode(id int64, labels []string, props map[string]interface{}) *Node {
    if props == nil {
        props = make(map[string]interface{})
    }
    return &Node{
        ID:         id,
        Labels:     labels,
        Properties: props,
    }
}

// HasLabel проверяет, есть ли у узла метка
func (n *Node) HasLabel(label string) bool {
    for _, l := range n.Labels {
        if l == label {
            return true
        }
    }
    return false
}

// GetProp возвращает свойство по имени
func (n *Node) GetProp(name string) (interface{}, bool) {
    val, ok := n.Properties[name]
    return val, ok
}

// Serialize сериализует узел в JSON байты
func (n *Node) Serialize() ([]byte, error) {
    data := map[string]interface{}{
        "id":         n.ID,
        "labels":     n.Labels,
        "properties": n.Properties,
    }
    return json.Marshal(data)
}

// DeserializeNode восстанавливает узел из байтов
func DeserializeNode(data []byte) (*Node, error) {
    var raw map[string]interface{}
    if err := json.Unmarshal(data, &raw); err != nil {
        return nil, err
    }

    id, _ := raw["id"].(float64)

    var labels []string
    if labelsRaw, ok := raw["labels"].([]interface{}); ok {
        for _, l := range labelsRaw {
            if s, ok := l.(string); ok {
                labels = append(labels, s)
            }
        }
    }

    props, _ := raw["properties"].(map[string]interface{})

    return NewNode(int64(id), labels, props), nil
}

func (n *Node) String() string {
    return fmt.Sprintf("Node[%d] labels=%v props=%v", n.ID, n.Labels, n.Properties)
}