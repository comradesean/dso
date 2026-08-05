// Package store is the persistence layer. It is backed by SQLite through
// modernc.org/sqlite, a pure-Go driver, so the server still builds with
// CGO_ENABLED=0 and stays a single static binary.
//
// Not everything is persisted, and that is deliberate. Blood messages are
// long-lived player-authored content and losing them on restart is painful.
// Bloodstains and ghosts are high-volume and inherently disposable — the
// reference keeps them memory-only by default too (BloodstainMemoryCacheOnly and
// GhostMemoryCacheOnly, both true in its RuntimeConfig) — so they stay in memory
// here rather than paying a write for data nobody misses.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// BloodMessage is one persisted message.
type BloodMessage struct {
	ID          uint32
	PlayerID    uint32
	CharacterID uint32
	AccountID   string
	AreaID      uint32
	CellID      uint32
	Data        []byte
	Rating      int64
}

// Store is the database handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and applies the schema.
// A path of ":memory:" gives an ephemeral database, which is what tests use.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}
	// The driver is not safe for unlimited concurrent writers; SQLite serializes
	// them anyway, so cap the pool rather than let contention surface as
	// "database is locked".
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// migrate creates the schema. Safe to run on every start.
func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS blood_messages (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id    INTEGER NOT NULL,
    character_id INTEGER NOT NULL,
    account_id   TEXT    NOT NULL,
    area_id      INTEGER NOT NULL,
    cell_id      INTEGER NOT NULL,
    data         BLOB    NOT NULL,
    rating       INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL DEFAULT (unixepoch())
);
-- Listings always filter by area and then by a set of cells, so this is the
-- query shape that matters.
CREATE INDEX IF NOT EXISTS idx_blood_messages_area_cell
    ON blood_messages(area_id, cell_id);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// AddBloodMessage inserts a message and returns its assigned id.
func (s *Store) AddBloodMessage(ctx context.Context, m *BloodMessage) (uint32, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO blood_messages
		   (player_id, character_id, account_id, area_id, cell_id, data, rating)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.PlayerID, m.CharacterID, m.AccountID, m.AreaID, m.CellID, m.Data, m.Rating)
	if err != nil {
		return 0, fmt.Errorf("store: insert blood message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: blood message id: %w", err)
	}
	return uint32(id), nil
}

// RemoveBloodMessage deletes a message.
func (s *Store) RemoveBloodMessage(ctx context.Context, id uint32) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM blood_messages WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete blood message %d: %w", id, err)
	}
	return nil
}

// GetBloodMessage returns one message by id.
func (s *Store) GetBloodMessage(ctx context.Context, id uint32) (*BloodMessage, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, player_id, character_id, account_id, area_id, cell_id, data, rating
		   FROM blood_messages WHERE id = ?`, id)
	m, err := scanBloodMessage(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: get blood message %d: %w", id, err)
	}
	return m, true, nil
}

// EvaluateBloodMessage increments a message's rating and returns the new total.
func (s *Store) EvaluateBloodMessage(ctx context.Context, id uint32) (int64, bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE blood_messages SET rating = rating + 1 WHERE id = ?`, id)
	if err != nil {
		return 0, false, fmt.Errorf("store: evaluate blood message %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return 0, false, nil
	}
	var rating int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT rating FROM blood_messages WHERE id = ?`, id).Scan(&rating); err != nil {
		return 0, false, fmt.Errorf("store: read rating %d: %w", id, err)
	}
	return rating, true, nil
}

// BloodMessagesInCells returns up to limit messages in the area, restricted to
// the given cells. An empty cells slice means any cell in the area.
func (s *Store) BloodMessagesInCells(ctx context.Context, areaID uint32, cells []uint32, limit int) ([]*BloodMessage, error) {
	q := `SELECT id, player_id, character_id, account_id, area_id, cell_id, data, rating
	        FROM blood_messages WHERE area_id = ?`
	args := []any{areaID}
	if len(cells) > 0 {
		q += ` AND cell_id IN (`
		for i, c := range cells {
			if i > 0 {
				q += `,`
			}
			q += `?`
			args = append(args, c)
		}
		q += `)`
	}
	// Newest first, so a busy area shows recent messages rather than whichever
	// happened to be inserted first.
	q += ` ORDER BY id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list blood messages: %w", err)
	}
	defer rows.Close()

	var out []*BloodMessage
	for rows.Next() {
		m, err := scanBloodMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan blood message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanBloodMessage(sc scanner) (*BloodMessage, error) {
	var m BloodMessage
	if err := sc.Scan(&m.ID, &m.PlayerID, &m.CharacterID, &m.AccountID,
		&m.AreaID, &m.CellID, &m.Data, &m.Rating); err != nil {
		return nil, err
	}
	return &m, nil
}
