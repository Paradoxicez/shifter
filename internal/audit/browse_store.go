package audit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Filter mirrors the URL-state schema; nil/empty fields = "no filter on this dimension".
// When From and To are both nil, ListCursor defaults to the last 7 days (D-33).
type Filter struct {
	From          *time.Time
	To            *time.Time
	UserID        *uuid.UUID
	EntityTypes   []string // empty = no filter
	Actions       []string // empty = no filter
	RequestIDLike *string
}

// Cursor is the row-comparison key for stable pagination (D-36).
// It encodes the (time, id) of the last row returned by the previous page.
type Cursor struct {
	Time time.Time `json:"t"`
	ID   uuid.UUID `json:"i"`
}

// EncodeCursor returns a URL-safe base64 JSON string; nil input → "".
func EncodeCursor(c *Cursor) string {
	if c == nil {
		return ""
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses a base64url-encoded cursor string.
// Returns nil for "" or any invalid input.
func DecodeCursor(s string) *Cursor {
	if s == "" {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil
	}
	return &c
}

// MustDecodeCursor is a test helper that panics on invalid input.
func MustDecodeCursor(s string) *Cursor {
	c := DecodeCursor(s)
	if c == nil {
		panic(fmt.Sprintf("audit: MustDecodeCursor: invalid cursor %q", s))
	}
	return c
}

// Row is a single audit log row as returned by the browse store.
// Includes the joined user email for display convenience.
type Row struct {
	ID         uuid.UUID
	Time       time.Time
	Action     string
	EntityType string
	EntityID   uuid.UUID
	Before     []byte
	After      []byte
	Notes      string
	RequestID  string
	UserID     *uuid.UUID
	UserEmail  string
}

// ListResult is the paged response from ListCursor.
type ListResult struct {
	Rows       []Row
	NextCursor string
	Total      int64
}

// Store holds the DB pool for audit browse queries.
// Constructed via NewStore.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new Store backed by the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const listPageSize = 100

// ListCursor returns up to 100 audit rows using stable row-comparison cursor
// pagination (D-36). When cursor is nil, the first page is returned.
//
// If Filter.From and Filter.To are both nil, a 7-day default window is applied
// (D-33: now()-7d .. now()).
func (s *Store) ListCursor(ctx context.Context, f Filter, cursor *Cursor) (*ListResult, error) {
	from, to := applyDefaultWindow(f.From, f.To)

	// Build arrays for ANY() filters; nil → no array filter.
	var entityTypeArr interface{}
	if len(f.EntityTypes) > 0 {
		entityTypeArr = f.EntityTypes
	}
	var actionArr interface{}
	if len(f.Actions) > 0 {
		actionArr = f.Actions
	}

	var cursorTime interface{}
	var cursorID interface{}
	if cursor != nil {
		cursorTime = cursor.Time
		cursorID = cursor.ID
	}

	const q = `
SELECT a.id, a.time, a.action, a.entity_type, a.entity_id,
       a.before, a.after, a.notes, a.request_id,
       a.user_id, COALESCE(u.email, '') AS user_email
FROM audit_log a
LEFT JOIN "user" u ON a.user_id = u.id
WHERE (
    $1::TIMESTAMPTZ IS NULL
    OR (a.time, a.id) < ($1::TIMESTAMPTZ, $2::UUID)
)
AND ($3::TIMESTAMPTZ IS NULL OR a.time >= $3)
AND ($4::TIMESTAMPTZ IS NULL OR a.time <  $4)
AND ($5::UUID IS NULL OR a.user_id = $5)
AND ($6::TEXT[] IS NULL OR a.entity_type = ANY($6))
AND ($7::TEXT[] IS NULL OR a.action = ANY($7))
AND ($8::TEXT IS NULL OR a.request_id ILIKE '%' || $8 || '%')
ORDER BY a.time DESC, a.id DESC
LIMIT 100
`

	rows, err := s.pool.Query(ctx, q,
		cursorTime,          // $1 cursor_time
		cursorID,            // $2 cursor_id
		from,                // $3 from
		to,                  // $4 to
		f.UserID,            // $5 user_id
		entityTypeArr,       // $6 entity_type[]
		actionArr,           // $7 action[]
		f.RequestIDLike,     // $8 request_id LIKE
	)
	if err != nil {
		return nil, fmt.Errorf("audit.ListCursor query: %w", err)
	}
	defer rows.Close()

	var result []Row
	for rows.Next() {
		var r Row
		var idStr, entityIDStr string
		var notesStr, requestIDStr, userIDStr *string
		if err := rows.Scan(
			&idStr, &r.Time, &r.Action, &r.EntityType, &entityIDStr,
			&r.Before, &r.After, &notesStr, &requestIDStr,
			&userIDStr, &r.UserEmail,
		); err != nil {
			return nil, fmt.Errorf("audit.ListCursor scan: %w", err)
		}
		r.ID, _ = uuid.Parse(idStr)
		r.EntityID, _ = uuid.Parse(entityIDStr)
		if notesStr != nil {
			r.Notes = *notesStr
		}
		if requestIDStr != nil {
			r.RequestID = *requestIDStr
		}
		if userIDStr != nil {
			uid, _ := uuid.Parse(*userIDStr)
			r.UserID = &uid
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit.ListCursor rows: %w", err)
	}

	// Build next cursor from the last row if we got a full page.
	var nextCursor string
	if len(result) == listPageSize {
		last := result[len(result)-1]
		nextCursor = EncodeCursor(&Cursor{Time: last.Time, ID: last.ID})
	}

	// Count is always fetched alongside the page.
	total, err := s.count(ctx, f)
	if err != nil {
		return nil, err
	}

	return &ListResult{
		Rows:       result,
		NextCursor: nextCursor,
		Total:      total,
	}, nil
}

// Count returns the total number of audit rows matching the filter.
// Uses the same WHERE clause as ListCursor (without cursor / ORDER BY / LIMIT).
// If From and To are both nil, the default 7-day window is applied.
func (s *Store) Count(ctx context.Context, f Filter) (int64, error) {
	return s.count(ctx, f)
}

func (s *Store) count(ctx context.Context, f Filter) (int64, error) {
	from, to := applyDefaultWindow(f.From, f.To)

	var entityTypeArr interface{}
	if len(f.EntityTypes) > 0 {
		entityTypeArr = f.EntityTypes
	}
	var actionArr interface{}
	if len(f.Actions) > 0 {
		actionArr = f.Actions
	}

	const q = `
SELECT count(*)
FROM audit_log a
WHERE ($1::TIMESTAMPTZ IS NULL OR a.time >= $1)
  AND ($2::TIMESTAMPTZ IS NULL OR a.time <  $2)
  AND ($3::UUID IS NULL OR a.user_id = $3)
  AND ($4::TEXT[] IS NULL OR a.entity_type = ANY($4))
  AND ($5::TEXT[] IS NULL OR a.action = ANY($5))
  AND ($6::TEXT IS NULL OR a.request_id ILIKE '%' || $6 || '%')
`

	var n int64
	err := s.pool.QueryRow(ctx, q,
		from,            // $1
		to,              // $2
		f.UserID,        // $3
		entityTypeArr,   // $4
		actionArr,       // $5
		f.RequestIDLike, // $6
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("audit.Count: %w", err)
	}
	return n, nil
}

// DistinctActions returns the sorted list of distinct action values in audit_log.
// Used by the filter dropdown.
func (s *Store) DistinctActions(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT action FROM audit_log ORDER BY action ASC`)
	if err != nil {
		return nil, fmt.Errorf("audit.DistinctActions: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DistinctEntityTypes returns the sorted list of distinct entity_type values.
func (s *Store) DistinctEntityTypes(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT entity_type FROM audit_log ORDER BY entity_type ASC`)
	if err != nil {
		return nil, fmt.Errorf("audit.DistinctEntityTypes: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var et string
		if err := rows.Scan(&et); err != nil {
			return nil, err
		}
		out = append(out, et)
	}
	return out, rows.Err()
}

// applyDefaultWindow returns the from/to values to use in queries.
// When both are nil, defaults to [now()-7d, now()] per D-33.
func applyDefaultWindow(from, to *time.Time) (interface{}, interface{}) {
	if from == nil && to == nil {
		now := time.Now().UTC()
		sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
		return sevenDaysAgo, now
	}
	return from, to
}
