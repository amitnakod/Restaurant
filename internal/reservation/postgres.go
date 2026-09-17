package reservation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const defaultPostgresDSN = "postgres://postgres:postgres@localhost:5432/restaurant"

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(dsn string) (*PostgresStore, error) {
	if dsn == "" {
		dsn = defaultPostgresDSN
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := initPostgresSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func initPostgresSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS restaurant_tables (
			id TEXT PRIMARY KEY,
			capacity INTEGER NOT NULL CHECK (capacity > 0)
		);
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		INSERT INTO restaurant_tables (id, capacity)
		VALUES ('table-1', 2), ('table-2', 4), ('table-3', 6), ('table-4', 8)
		ON CONFLICT (id) DO NOTHING;
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS reservations (
			id TEXT PRIMARY KEY,
			table_id TEXT NOT NULL,
			date TEXT NOT NULL,
			slot TEXT NOT NULL,
			guest_count INTEGER NOT NULL CHECK (guest_count > 0),
			customer_name TEXT NOT NULL,
			customer_email TEXT NOT NULL,
			customer_phone TEXT,
			special_requests TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT fk_reservation_table FOREIGN KEY (table_id) REFERENCES restaurant_tables(id)
		);
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_reservations_lookup ON reservations (table_id, date, slot);
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE SEQUENCE IF NOT EXISTS reservation_id_seq;
	`); err != nil {
		return err
	}
	return nil
}

func (s *PostgresStore) Tables() []Table {
	rows, err := s.db.Query(`SELECT id, capacity FROM restaurant_tables ORDER BY id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make([]Table, 0)
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.ID, &t.Capacity); err != nil {
			return nil
		}
		result = append(result, t)
	}
	return result
}

func (s *PostgresStore) Available(date string, partySize int) ([]Availability, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil || partySize < 1 {
		return nil, errors.New("date must be YYYY-MM-DD and party_size must be positive")
	}

	tables := s.Tables()
	result := make([]Availability, 0, len(Slots))
	for _, slot := range Slots {
		a := Availability{Slot: slot}
		for _, table := range tables {
			booked := s.BookedSeats(table.ID, date, slot)
			remaining := table.Capacity - booked
			if remaining >= partySize {
				a.Tables = append(a.Tables, TableAvailability{
					TableID:        table.ID,
					Capacity:       table.Capacity,
					BookedSeats:    booked,
					RemainingSeats: remaining,
				})
			}
		}
		sort.Slice(a.Tables, func(i, j int) bool { return a.Tables[i].TableID < a.Tables[j].TableID })
		result = append(result, a)
	}
	return result, nil
}

func (s *PostgresStore) TableByID(id string) (Table, bool) {
	var table Table
	err := s.db.QueryRow(`SELECT id, capacity FROM restaurant_tables WHERE id = $1`, id).Scan(&table.ID, &table.Capacity)
	if err != nil {
		return Table{}, false
	}
	return table, true
}

func (s *PostgresStore) BookedSeats(tableID, date, slot string) int {
	var total int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(guest_count), 0) FROM reservations WHERE table_id = $1 AND date = $2 AND slot = $3`, tableID, date, slot).Scan(&total)
	if err != nil {
		return 0
	}
	return total
}

func (s *PostgresStore) GetReservation(id string) (Reservation, bool) {
	var r Reservation
	var customerName, customerEmail, customerPhone, specialRequests sql.NullString
	err := s.db.QueryRow(`
		SELECT id, table_id, date, slot, guest_count, customer_name, customer_email, customer_phone, special_requests, created_at
		FROM reservations WHERE id = $1
	`, id).Scan(
		&r.ID,
		&r.TableID,
		&r.Date,
		&r.Slot,
		&r.GuestCount,
		&customerName,
		&customerEmail,
		&customerPhone,
		&specialRequests,
		&r.CreatedAt,
	)
	if err != nil {
		return Reservation{}, false
	}
	r.Customer = Customer{Name: customerName.String, Email: customerEmail.String, Phone: customerPhone.String}
	r.SpecialRequests = specialRequests.String
	return r, true
}

func (s *PostgresStore) Create(r Reservation) (Reservation, error) {
	tx, err := s.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback()

	var capacity int
	if err := tx.QueryRow(`SELECT capacity FROM restaurant_tables WHERE id = $1 FOR UPDATE`, r.TableID).Scan(&capacity); err != nil {
		if err == sql.ErrNoRows {
			return Reservation{}, ErrNotFound
		}
		return Reservation{}, err
	}

	var booked int
	if err := tx.QueryRow(`SELECT COALESCE(SUM(guest_count), 0) FROM reservations WHERE table_id = $1 AND date = $2 AND slot = $3`, r.TableID, r.Date, r.Slot).Scan(&booked); err != nil {
		return Reservation{}, err
	}
	if booked+r.GuestCount > capacity {
		return Reservation{}, ErrNoCapacity
	}

	var id string
	if err := tx.QueryRow(`SELECT 'res-' || LPAD(CAST(nextval('reservation_id_seq') AS text), 6, '0')`).Scan(&id); err != nil {
		return Reservation{}, err
	}
	r.ID = id
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}

	if _, err := tx.Exec(`
		INSERT INTO reservations (id, table_id, date, slot, guest_count, customer_name, customer_email, customer_phone, special_requests, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, r.ID, r.TableID, r.Date, r.Slot, r.GuestCount, r.Customer.Name, r.Customer.Email, r.Customer.Phone, r.SpecialRequests, r.CreatedAt); err != nil {
		return Reservation{}, err
	}

	if err := tx.Commit(); err != nil {
		return Reservation{}, err
	}
	return r, nil
}

func (s *PostgresStore) Cancel(id string) error {
	result, err := s.db.Exec(`DELETE FROM reservations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *PostgresStore) validate() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres store not initialized")
	}
	return nil
}
