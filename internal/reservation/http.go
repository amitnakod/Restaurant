package reservation

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) http.Handler { return Handler{service} }
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tables":
		writeJSON(w, http.StatusOK, map[string]any{"tables": h.service.Tables()})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/availability":
		h.availability(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/reservations":
		h.create(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/reservations/"):
		h.cancel(w, strings.TrimPrefix(r.URL.Path, "/api/v1/reservations/"))
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
	}
}
func (h Handler) availability(w http.ResponseWriter, r *http.Request) {
	party := 0
	if _, err := fmtSscanf(r.URL.Query().Get("party_size"), &party); err != nil {
		writeJSON(w, 400, map[string]string{"error": "party_size must be a positive integer"})
		return
	}
	slots, err := h.service.Available(r.URL.Query().Get("date"), party)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"date": r.URL.Query().Get("date"), "party_size": party, "slots": slots})
}
func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	var input Reservation
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	created, err := h.service.Create(input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 201, created)
}
func (h Handler) cancel(w http.ResponseWriter, id string) {
	if id == "" {
		writeJSON(w, 404, map[string]string{"error": "route not found"})
		return
	}
	if err := h.service.Cancel(id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func writeServiceError(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if errors.Is(err, ErrNotFound) {
		code = 404
	}
	if errors.Is(err, ErrNoCapacity) {
		code = 409
	}
	if errors.Is(err, ErrCancellationWindow) {
		code = 422
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func fmtSscanf(v string, out *int) (int, error) {
	if v == "" {
		return 0, errors.New("empty")
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return 0, errors.New("not integer")
		}
		n = n*10 + int(c-'0')
	}
	*out = n
	return 1, nil
}
