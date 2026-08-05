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

// firstMessageID is where blood-message ids start.
//
// Ids must NEVER be reused, and not merely within one database: clients cache
// evaluation state by message id across sessions, so handing a fresh message an
// id a client has already rated makes it grey out the rate option for a message
// it has never seen. That is exactly what happened when this store replaced an
// in-memory one and numbering restarted at 1.
//
// AUTOINCREMENT (as opposed to a plain INTEGER PRIMARY KEY) guarantees ids are
// never reused within this database even after deletes. Starting well above the
// small ids any earlier in-memory run handed out keeps us clear of what existing
// clients already remember.
const firstMessageID = 100000

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

-- Cumulative world statistics. A generic table rather than a total_deaths
-- column: RequestNotifyKillEnemy and RequestNotifyBuyItem are the same shape of
-- statistic and already arrive unhandled.
CREATE TABLE IF NOT EXISTS counters (
    name  TEXT    PRIMARY KEY,
    value INTEGER NOT NULL DEFAULT 0
);

-- Power-stone leaderboard. One row per character; the client submits score
-- increments and an opaque blob it renders itself.
--
-- Ranks are NOT stored. They are derived on read, so they cannot go stale
-- against the scores they describe — the reference keeps them as columns and has
-- to maintain them.
CREATE TABLE IF NOT EXISTS power_stone_rankings (
    character_id INTEGER PRIMARY KEY,
    player_id    INTEGER NOT NULL,
    score        INTEGER NOT NULL DEFAULT 0,
    data         BLOB    NOT NULL,
    updated_at   INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS idx_power_stone_score
    ON power_stone_rankings(score DESC);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}

	// Push the id sequence past the low range, once, without disturbing a
	// database that has already issued higher ids. sqlite_sequence holds the
	// highest id ever used for an AUTOINCREMENT table.
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sqlite_sequence(name, seq) SELECT 'blood_messages', ?
		   WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'blood_messages')`,
		firstMessageID-1); err != nil {
		return fmt.Errorf("store: seed message id sequence: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sqlite_sequence SET seq = ? WHERE name = 'blood_messages' AND seq < ?`,
		firstMessageID-1, firstMessageID-1); err != nil {
		return fmt.Errorf("store: raise message id sequence: %w", err)
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

// AddCounter adds delta to a named counter, creating it if absent, and returns
// the new value.
//
// Counters are cumulative world statistics — low-volume and meaningless if they
// reset, which is why these are persisted where bloodstains and ghosts are not.
func (s *Store) AddCounter(ctx context.Context, name string, delta int64) (int64, error) {
	var value int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO counters(name, value) VALUES(?, ?)
		   ON CONFLICT(name) DO UPDATE SET value = value + excluded.value
		 RETURNING value`, name, delta).Scan(&value)
	if err != nil {
		return 0, fmt.Errorf("store: add counter %q: %w", name, err)
	}
	return value, nil
}

// Counter returns a named counter, or 0 if it has never been set.
func (s *Store) Counter(ctx context.Context, name string) (int64, error) {
	var value int64
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM counters WHERE name = ?`, name).Scan(&value)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: read counter %q: %w", name, err)
	}
	return value, nil
}

// PowerStoneRanking is one leaderboard row, with its ranks derived at read time.
type PowerStoneRanking struct {
	CharacterID uint32
	PlayerID    uint32
	Score       int64
	Data        []byte
	// SerialRank is the unique 1-based position in the board.
	SerialRank uint32
	// Rank is the competition rank: tied scores share it, so two players on the
	// same score are both "2nd" and the next is 4th.
	Rank uint32
}

// rankingSelect derives both rank flavours in one pass. Ordering by score then
// character id keeps SerialRank stable and deterministic across equal scores;
// Rank deliberately orders by score alone so ties genuinely tie.
const rankingSelect = `
SELECT character_id, player_id, score, data,
       ROW_NUMBER() OVER (ORDER BY score DESC, character_id ASC) AS serial_rank,
       RANK()       OVER (ORDER BY score DESC)                   AS rank
  FROM power_stone_rankings`

// AddPowerStoneScore applies a score increment for a character and returns the
// new total. The blob is replaced wholesale — it is the client's own rendering of
// the entry and only the latest matters.
func (s *Store) AddPowerStoneScore(ctx context.Context, characterID, playerID uint32, increment int64, data []byte) (int64, error) {
	var score int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO power_stone_rankings(character_id, player_id, score, data)
		   VALUES(?, ?, ?, ?)
		 ON CONFLICT(character_id) DO UPDATE SET
		   score      = score + excluded.score,
		   player_id  = excluded.player_id,
		   data       = excluded.data,
		   updated_at = unixepoch()
		 RETURNING score`,
		characterID, playerID, increment, data).Scan(&score)
	if err != nil {
		return 0, fmt.Errorf("store: add power stone score for character %d: %w", characterID, err)
	}
	return score, nil
}

// PowerStoneRankings returns a page of the board. offset is 1-based, matching
// what the client sends.
func (s *Store) PowerStoneRankings(ctx context.Context, offset, count uint32) ([]*PowerStoneRanking, error) {
	if offset > 0 {
		offset--
	}
	rows, err := s.db.QueryContext(ctx,
		rankingSelect+` ORDER BY serial_rank ASC LIMIT ? OFFSET ?`, count, offset)
	if err != nil {
		return nil, fmt.Errorf("store: list power stone rankings: %w", err)
	}
	defer rows.Close()

	var out []*PowerStoneRanking
	for rows.Next() {
		r, err := scanRanking(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan power stone ranking: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PowerStoneRankingFor returns one character's entry, or false if it has none.
func (s *Store) PowerStoneRankingFor(ctx context.Context, characterID uint32) (*PowerStoneRanking, bool, error) {
	// The ranks come from the window over the whole board, so the filter has to
	// be applied outside it — filtering first would rank the character against
	// itself and always return 1.
	row := s.db.QueryRowContext(ctx,
		`SELECT character_id, player_id, score, data, serial_rank, rank
		   FROM (`+rankingSelect+`) WHERE character_id = ?`, characterID)
	r, err := scanRanking(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: get power stone ranking %d: %w", characterID, err)
	}
	return r, true, nil
}

// PowerStoneRankingCount is the number of entries on the board.
func (s *Store) PowerStoneRankingCount(ctx context.Context) (uint32, error) {
	var n uint32
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM power_stone_rankings`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count power stone rankings: %w", err)
	}
	return n, nil
}

func scanRanking(sc scanner) (*PowerStoneRanking, error) {
	var r PowerStoneRanking
	if err := sc.Scan(&r.CharacterID, &r.PlayerID, &r.Score, &r.Data,
		&r.SerialRank, &r.Rank); err != nil {
		return nil, err
	}
	return &r, nil
}
