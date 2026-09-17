package reservation

import (
	"sync"
	"testing"
	"time"
)

func testService() *Service {
	return NewService(NewMemoryStore(), func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.Local) })
}
func reservationFor(guests int) Reservation {
	return Reservation{TableID: "table-2", Date: "2030-01-02", Slot: "18:00", GuestCount: guests, Customer: Customer{Name: "Ada", Email: "ada@example.com"}}
}
func TestAvailabilityIncludesPartiallyFilledTable(t *testing.T) {
	s := testService()
	if _, err := s.Create(reservationFor(2)); err != nil {
		t.Fatal(err)
	}
	slots, err := s.Available("2030-01-02", 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range slots[2].Tables {
		if table.TableID == "table-2" && table.BookedSeats == 2 && table.RemainingSeats == 2 {
			return
		}
	}
	t.Fatal("expected partially filled table-2")
}
func TestConcurrentBookingsNeverOverbook(t *testing.T) {
	s := testService()
	var wg sync.WaitGroup
	success := 0
	var mu sync.Mutex
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Create(reservationFor(1)); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 4 {
		t.Fatalf("got %d successful bookings, want 4", success)
	}
}
func TestCancellationWindow(t *testing.T) {
	s := testService()
	r, err := s.Create(reservationFor(1))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Cancel(r.ID); err != nil {
		t.Fatalf("expected cancellation, got %v", err)
	}
	soon := NewService(NewMemoryStore(), func() time.Time { return time.Date(2030, 1, 2, 17, 0, 0, 0, time.Local) })
	r, err = soon.Create(reservationFor(1))
	if err != nil {
		t.Fatal(err)
	}
	if err = soon.Cancel(r.ID); err != ErrCancellationWindow {
		t.Fatalf("got %v", err)
	}
}
