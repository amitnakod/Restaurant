package reservation

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrNoCapacity         = errors.New("table does not have enough remaining capacity")
	ErrCancellationWindow = errors.New("reservations can only be cancelled at least 2 hours before their slot")
	ErrInvalidSlot        = errors.New("invalid time slot")
)

var Slots = []string{"12:00", "14:00", "18:00", "20:00"}

type Clock func() time.Time

var DefaultClock Clock = time.Now

type Table struct {
	ID       string `json:"id"`
	Capacity int    `json:"capacity"`
}
type Customer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}
type Reservation struct {
	ID              string    `json:"id"`
	TableID         string    `json:"table_id"`
	Date            string    `json:"date"`
	Slot            string    `json:"slot"`
	Customer        Customer  `json:"customer"`
	GuestCount      int       `json:"guest_count"`
	SpecialRequests string    `json:"special_requests,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
type Availability struct {
	Slot   string              `json:"slot"`
	Tables []TableAvailability `json:"tables"`
}
type TableAvailability struct {
	TableID        string `json:"table_id"`
	Capacity       int    `json:"capacity"`
	BookedSeats    int    `json:"booked_seats"`
	RemainingSeats int    `json:"remaining_seats"`
}

type Store interface {
	Tables() []Table
	Available(date string, partySize int) ([]Availability, error)
	Create(Reservation) (Reservation, error)
	Cancel(string) error
	GetReservation(string) (Reservation, bool)
}

type MemoryStore struct {
	mu           sync.Mutex
	tables       map[string]Table
	reservations map[string]Reservation
	nextID       int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tables: map[string]Table{"table-1": {"table-1", 2}, "table-2": {"table-2", 4}, "table-3": {"table-3", 6}, "table-4": {"table-4", 8}}, reservations: make(map[string]Reservation)}
}

type Service struct {
	store Store
	now   Clock
}

func NewService(store Store, now Clock) *Service { return &Service{store: store, now: now} }
func validSlot(slot string) bool {
	for _, s := range Slots {
		if s == slot {
			return true
		}
	}
	return false
}

func (s *Service) Tables() []Table {
	return s.store.Tables()
}
func (s *Service) Available(date string, partySize int) ([]Availability, error) {
	return s.store.Available(date, partySize)
}
func (s *Service) Create(r Reservation) (Reservation, error) {
	if _, err := time.Parse("2006-01-02", r.Date); err != nil {
		return Reservation{}, errors.New("date must be YYYY-MM-DD")
	}
	if !validSlot(r.Slot) {
		return Reservation{}, ErrInvalidSlot
	}
	if r.GuestCount < 1 {
		return Reservation{}, errors.New("guest_count must be positive")
	}
	if r.Customer.Name == "" || r.Customer.Email == "" {
		return Reservation{}, errors.New("customer name and email are required")
	}
	r.CreatedAt = s.now().UTC()
	return s.store.Create(r)
}
func (s *Service) Cancel(id string) error {
	r, ok := s.store.GetReservation(id)
	if !ok {
		return ErrNotFound
	}
	starts, err := time.ParseInLocation("2006-01-02 15:04", r.Date+" "+r.Slot, time.Local)
	if err != nil {
		return err
	}
	if s.now().After(starts.Add(-2 * time.Hour)) {
		return ErrCancellationWindow
	}
	return s.store.Cancel(id)
}

func (m *MemoryStore) Tables() []Table {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Table, 0, len(m.tables))
	for _, t := range m.tables {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *MemoryStore) Available(date string, partySize int) ([]Availability, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil || partySize < 1 {
		return nil, errors.New("date must be YYYY-MM-DD and party_size must be positive")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Availability, 0, len(Slots))
	for _, slot := range Slots {
		a := Availability{Slot: slot}
		for _, table := range m.tables {
			booked := m.bookedSeats(table.ID, date, slot)
			remaining := table.Capacity - booked
			if remaining >= partySize {
				a.Tables = append(a.Tables, TableAvailability{TableID: table.ID, Capacity: table.Capacity, BookedSeats: booked, RemainingSeats: remaining})
			}
		}
		sort.Slice(a.Tables, func(i, j int) bool { return a.Tables[i].TableID < a.Tables[j].TableID })
		result = append(result, a)
	}
	return result, nil
}

func (m *MemoryStore) bookedSeats(tableID, date, slot string) int {
	total := 0
	for _, r := range m.reservations {
		if r.TableID == tableID && r.Date == date && r.Slot == slot {
			total += r.GuestCount
		}
	}
	return total
}

func (m *MemoryStore) Create(r Reservation) (Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	table, ok := m.tables[r.TableID]
	if !ok {
		return Reservation{}, ErrNotFound
	}
	if m.bookedSeats(r.TableID, r.Date, r.Slot)+r.GuestCount > table.Capacity {
		return Reservation{}, ErrNoCapacity
	}
	m.nextID++
	r.ID = fmt.Sprintf("res-%06d", m.nextID)
	m.reservations[r.ID] = r
	return r, nil
}

func (m *MemoryStore) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reservations[id]; !ok {
		return ErrNotFound
	}
	delete(m.reservations, id)
	return nil
}

func (m *MemoryStore) GetReservation(id string) (Reservation, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reservations[id]
	return r, ok
}
