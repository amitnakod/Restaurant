package reservation

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBookingAndAvailabilityHTTP(t *testing.T) {
	h := NewHandler(testService())
	body := []byte(`{"table_id":"table-2","date":"2030-01-02","slot":"18:00","guest_count":2,"customer":{"name":"Ada","email":"ada@example.com"}}`)
	create := httptest.NewRecorder()
	h.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/reservations", bytes.NewReader(body)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", create.Code, create.Body.String())
	}
	availability := httptest.NewRecorder()
	h.ServeHTTP(availability, httptest.NewRequest(http.MethodGet, "/api/v1/availability?date=2030-01-02&party_size=2", nil))
	if availability.Code != http.StatusOK {
		t.Fatalf("availability status = %d: %s", availability.Code, availability.Body.String())
	}
	if !bytes.Contains(availability.Body.Bytes(), []byte(`"remaining_seats":2`)) {
		t.Fatalf("expected partial availability: %s", availability.Body.String())
	}
}
