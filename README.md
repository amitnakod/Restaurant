# Restaurant Table Reservation API

A dependency-free Go REST service implementing the brief's advanced scope: fixed slots, capacity-aware shared tables, safe concurrent booking, customer details, special requests, and cancellation.

## Run

Make sure PostgreSQL is running and reachable at the configured connection string.

```powershell
$env:DATABASE_URL="postgres://postgres:postgres@localhost:5432/restaurant"
go test ./...
go run ./cmd/server
```

The server listens on `http://localhost:8080` (override with `PORT`). Seeded tables have capacities 2, 4, 6, and 8.

## API

`GET /health`

`GET /api/v1/tables`

`GET /api/v1/availability?date=2030-01-02&party_size=3` returns every fixed slot and only tables whose remaining capacity accommodates the party. A partially booked table is included with `booked_seats` and `remaining_seats`.

`POST /api/v1/reservations`

```json
{
  "table_id": "table-2",
  "date": "2030-01-02",
  "slot": "18:00",
  "guest_count": 2,
  "customer": {"name": "Ada Lovelace", "email": "ada@example.com", "phone": "+1-555-0100"},
  "special_requests": "Window seat if possible"
}
```

`DELETE /api/v1/reservations/{id}` returns `204` on success.

## Decisions and assumptions

- Slots are `12:00`, `14:00`, `18:00`, and `20:00`; each reservation occupies one slot.
- Parties may share a table. A booking is accepted only when the sum of guests for that table/date/slot does not exceed its capacity.
- The in-memory repository serializes the check-and-create operation with one mutex, making concurrent booking attempts atomic. It is intentionally process-local; production deployment should replace it with a database transaction/row lock or atomic conditional update.
- Cancellations are allowed until two hours before the local slot start time. The API returns `422` inside that window.
- Validation errors return `400`, a missing table/reservation returns `404`, and capacity conflicts return `409`.
