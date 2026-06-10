// pkg/executor/types.go
package executor

import "encoding/json"

// QueryResult represents the structured result of an SQL execution.
type QueryResult struct {
	Type         string          `json:"type"`                     // "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "CREATE_INDEX", "ERROR"
	Columns      []string        `json:"columns,omitempty"`        // For SELECT
	Rows         [][]interface{} `json:"rows,omitempty"`           // For SELECT
	AffectedRows int64           `json:"affected_rows,omitempty"`  // For INSERT, UPDATE, DELETE, CREATE, CREATE_INDEX
	LastInsertID int64           `json:"last_insert_id,omitempty"` // For INSERT
	Error        string          `json:"error,omitempty"`          // If an error occurred
}

// FormatResult marshals the QueryResult to a JSON string.
func (qr *QueryResult) FormatResult() (string, error) {
	jsonData, err := json.Marshal(qr)
	if err != nil {
		// Return an error result as JSON if marshaling fails
		errorResult := QueryResult{Type: "ERROR", Error: "Failed to marshal QueryResult: " + err.Error()}
		errorJSON, _ := json.Marshal(errorResult) // This marshal should ideally succeed
		return string(errorJSON), nil
	}
	return string(jsonData), nil
}

// FormatResultPretty marshals the QueryResult to an indented JSON string (useful for debugging).
func (qr *QueryResult) FormatResultPretty() (string, error) {
	jsonData, err := json.MarshalIndent(qr, "", "  ")
	if err != nil {
		// Return an error result as JSON if marshaling fails
		errorResult := QueryResult{Type: "ERROR", Error: "Failed to marshal QueryResult: " + err.Error()}
		errorJSON, _ := json.MarshalIndent(errorResult, "", "  ") // This marshal should ideally succeed
		return string(errorJSON), nil
	}
	return string(jsonData), nil
}
