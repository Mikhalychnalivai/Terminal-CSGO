package stats

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Writer handles batch writing of game events to the database.
type Writer struct {
	db                 *sql.DB
	stmtInsertShot     *sql.Stmt
	stmtInsertKill     *sql.Stmt
	stmtUpsertPlayer   *sql.Stmt
	stmtInsertSession  *sql.Stmt
	stmtUpdateSession  *sql.Stmt
	stmtInsertRoom     *sql.Stmt
	stmtUpdateRoom     *sql.Stmt
}

// Event represents a generic game event for batch processing.
type Event struct {
	Type string      // "shot", "kill", "session", "room"
	Data interface{} // ShotEvent, KillEvent, SessionEvent, or RoomEvent
}

// ShotEvent represents a weapon fire event.
type ShotEvent struct {
	PlayerID   string
	RoomID     string
	WeaponType int
	Hit        bool
	Timestamp  time.Time
}

// KillEvent represents a kill event.
type KillEvent struct {
	KillerID   string
	VictimID   string
	RoomID     string
	WeaponType int
	Timestamp  time.Time
}

// SessionEvent represents a session start or end event.
type SessionEvent struct {
	PlayerID  string
	RoomID    string
	MapID     string
	EventType string // "start" or "end"
	Timestamp time.Time
}

// RoomEvent represents a room lifecycle event.
type RoomEvent struct {
	RoomID    string
	MapID     string
	EventType string // "create" or "stop"
	Timestamp time.Time
}

// NewWriter creates a new Writer with prepared statements.
func NewWriter(db *Database) (*Writer, error) {
	w := &Writer{db: db.conn}

	var err error

	// Prepare shot insertion statement
	w.stmtInsertShot, err = db.conn.Prepare(`
		INSERT INTO shots (player_id, room_id, weapon_type, hit, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare shot insert: %w", err)
	}

	// Prepare kill insertion statement
	w.stmtInsertKill, err = db.conn.Prepare(`
		INSERT INTO kills (killer_id, victim_id, room_id, weapon_type, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare kill insert: %w", err)
	}

	// Prepare player upsert statement (insert or update aggregates)
	w.stmtUpsertPlayer, err = db.conn.Prepare(`
		INSERT INTO players (player_id, last_seen, total_kills, total_deaths, shots_fired, shots_hit, pistol_kills, rifle_kills)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(player_id) DO UPDATE SET
			last_seen = excluded.last_seen,
			total_kills = total_kills + excluded.total_kills,
			total_deaths = total_deaths + excluded.total_deaths,
			shots_fired = shots_fired + excluded.shots_fired,
			shots_hit = shots_hit + excluded.shots_hit,
			pistol_kills = pistol_kills + excluded.pistol_kills,
			rifle_kills = rifle_kills + excluded.rifle_kills
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare player upsert: %w", err)
	}

	// Prepare session insertion statement
	w.stmtInsertSession, err = db.conn.Prepare(`
		INSERT INTO sessions (player_id, room_id, map_id, start_time)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare session insert: %w", err)
	}

	// Prepare session update statement (for session end)
	w.stmtUpdateSession, err = db.conn.Prepare(`
		UPDATE sessions
		SET end_time = ?, duration_seconds = ?
		WHERE player_id = ? AND room_id = ? AND end_time IS NULL
		ORDER BY start_time DESC
		LIMIT 1
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare session update: %w", err)
	}

	// Prepare room insertion statement
	w.stmtInsertRoom, err = db.conn.Prepare(`
		INSERT OR IGNORE INTO rooms (room_id, map_id, created_at)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare room insert: %w", err)
	}

	// Prepare room update statement (for room stop)
	w.stmtUpdateRoom, err = db.conn.Prepare(`
		UPDATE rooms
		SET stopped_at = ?
		WHERE room_id = ? AND stopped_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare room update: %w", err)
	}

	return w, nil
}

// Close closes all prepared statements.
func (w *Writer) Close() error {
	var errs []error
	if w.stmtInsertShot != nil {
		if err := w.stmtInsertShot.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if w.stmtInsertKill != nil {
		if err := w.stmtInsertKill.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if w.stmtUpsertPlayer != nil {
		if err := w.stmtUpsertPlayer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if w.stmtInsertSession != nil {
		if err := w.stmtInsertSession.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if w.stmtUpdateSession != nil {
		if err := w.stmtUpdateSession.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if w.stmtInsertRoom != nil {
		if err := w.stmtInsertRoom.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if w.stmtUpdateRoom != nil {
		if err := w.stmtUpdateRoom.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing statements: %v", errs)
	}
	return nil
}

// WriteBatch processes a batch of events in a single transaction.
func (w *Writer) WriteBatch(events []Event) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := w.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, event := range events {
		switch event.Type {
		case "shot":
			if err := w.processShot(tx, event.Data.(ShotEvent)); err != nil {
				log.Printf("stats: failed to process shot event: %v", err)
				// Continue processing other events
			}

		case "kill":
			if err := w.processKill(tx, event.Data.(KillEvent)); err != nil {
				log.Printf("stats: failed to process kill event: %v", err)
			}

		case "session":
			if err := w.processSession(tx, event.Data.(SessionEvent)); err != nil {
				log.Printf("stats: failed to process session event: %v", err)
			}

		case "room":
			if err := w.processRoom(tx, event.Data.(RoomEvent)); err != nil {
				log.Printf("stats: failed to process room event: %v", err)
			}

		default:
			log.Printf("stats: unknown event type: %s", event.Type)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// processShot inserts a shot event and updates player aggregates.
func (w *Writer) processShot(tx *sql.Tx, shot ShotEvent) error {
	// Ensure player exists first (upsert with current timestamp)
	hitCount := 0
	if shot.Hit {
		hitCount = 1
	}

	_, err := tx.Stmt(w.stmtUpsertPlayer).Exec(
		shot.PlayerID, shot.Timestamp, 0, 0, 1, hitCount, 0, 0,
	)
	if err != nil {
		return fmt.Errorf("failed to update player stats: %w", err)
	}

	// Insert shot record
	_, err = tx.Stmt(w.stmtInsertShot).Exec(
		shot.PlayerID, shot.RoomID, shot.WeaponType, shot.Hit, shot.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to insert shot: %w", err)
	}

	return nil
}

// processKill inserts a kill event and updates killer/victim aggregates.
func (w *Writer) processKill(tx *sql.Tx, kill KillEvent) error {
	// Update killer stats first (ensure player exists)
	pistolKills := 0
	rifleKills := 0
	if kill.WeaponType == 1 {
		pistolKills = 1
	} else if kill.WeaponType == 2 {
		rifleKills = 1
	}

	_, err := tx.Stmt(w.stmtUpsertPlayer).Exec(
		kill.KillerID, kill.Timestamp, 1, 0, 0, 0, pistolKills, rifleKills,
	)
	if err != nil {
		return fmt.Errorf("failed to update killer stats: %w", err)
	}

	// Update victim stats (ensure player exists)
	_, err = tx.Stmt(w.stmtUpsertPlayer).Exec(
		kill.VictimID, kill.Timestamp, 0, 1, 0, 0, 0, 0,
	)
	if err != nil {
		return fmt.Errorf("failed to update victim stats: %w", err)
	}

	// Insert kill record
	_, err = tx.Stmt(w.stmtInsertKill).Exec(
		kill.KillerID, kill.VictimID, kill.RoomID, kill.WeaponType, kill.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to insert kill: %w", err)
	}

	return nil
}

// processSession handles session start/end events.
func (w *Writer) processSession(tx *sql.Tx, session SessionEvent) error {
	if session.EventType == "start" {
		// Insert new session
		_, err := tx.Stmt(w.stmtInsertSession).Exec(
			session.PlayerID, session.RoomID, session.MapID, session.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("failed to insert session: %w", err)
		}

		// Ensure player record exists
		_, err = tx.Stmt(w.stmtUpsertPlayer).Exec(
			session.PlayerID, session.Timestamp, 0, 0, 0, 0, 0, 0,
		)
		if err != nil {
			return fmt.Errorf("failed to ensure player exists: %w", err)
		}

	} else if session.EventType == "end" {
		// Find matching session start and calculate duration
		var startTime time.Time
		err := tx.QueryRow(`
			SELECT start_time FROM sessions
			WHERE player_id = ? AND room_id = ? AND end_time IS NULL
			ORDER BY start_time DESC
			LIMIT 1
		`, session.PlayerID, session.RoomID).Scan(&startTime)

		if err == sql.ErrNoRows {
			// No matching session start found, skip
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to find session start: %w", err)
		}

		duration := int(session.Timestamp.Sub(startTime).Seconds())
		if duration < 0 {
			duration = 0
		}

		// Update session with end time and duration
		_, err = tx.Stmt(w.stmtUpdateSession).Exec(
			session.Timestamp, duration, session.PlayerID, session.RoomID,
		)
		if err != nil {
			return fmt.Errorf("failed to update session: %w", err)
		}

		// Update player total playtime
		_, err = tx.Exec(`
			UPDATE players
			SET total_playtime_seconds = total_playtime_seconds + ?
			WHERE player_id = ?
		`, duration, session.PlayerID)
		if err != nil {
			return fmt.Errorf("failed to update playtime: %w", err)
		}
	}

	return nil
}

// processRoom handles room create/stop events.
func (w *Writer) processRoom(tx *sql.Tx, room RoomEvent) error {
	if room.EventType == "create" {
		_, err := tx.Stmt(w.stmtInsertRoom).Exec(
			room.RoomID, room.MapID, room.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("failed to insert room: %w", err)
		}

	} else if room.EventType == "stop" {
		_, err := tx.Stmt(w.stmtUpdateRoom).Exec(
			room.Timestamp, room.RoomID,
		)
		if err != nil {
			return fmt.Errorf("failed to update room: %w", err)
		}
	}

	return nil
}
