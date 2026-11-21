package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON answers 200 with value encoded as JSON.
func writeJSON(w http.ResponseWriter, value any) {
	writeJSONStatus(w, http.StatusOK, value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		body = []byte(`{"error":{"kind":"internal"}}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
