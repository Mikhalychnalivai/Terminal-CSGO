package stats

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Database manages SQLite connection and schema for game statistics.
type Database struct {
	conn *sql.DB
	path string
}

// NewDatabase creates a new Database instance and initializes schema.
// Creates the database file and parent directories if they don't exist.
func NewDatabase(dbPath string) (*Database, error) {
	// Create directory if not exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open SQLite connection
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)

	db := &Database{
		conn: conn,
		path: dbPath,
	}

	// Initialize schema
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Printf("statistics database initialized at %s", dbPath)
	return db, nil
}

// Close gracefully closes the database connection.
func (db *Database) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// initSchema creates all tables, indexes, and schema version tracking.
func (db *Database) initSchema() error {
	// Enable foreign keys
	if _, err := db.conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Schema version tracking
	if _, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	// Check current schema version
	var currentVersion int
	err := db.conn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to query schema version: %w", err)
	}

	// Apply migrations if needed
	if currentVersion == 0 {
		if err := db.applyInitialSchema(); err != nil {
			return fmt.Errorf("failed to apply initial schema: %w", err)
		}
	}

	return nil
}

// applyInitialSchema creates all tables and indexes for version 1.
func (db *Database) applyInitialSchema() error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Players table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS players (
			player_id TEXT PRIMARY KEY,
			first_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			total_kills INTEGER DEFAULT 0,
			total_deaths INTEGER DEFAULT 0,
			shots_fired INTEGER DEFAULT 0,
			shots_hit INTEGER DEFAULT 0,
			pistol_kills INTEGER DEFAULT 0,
			rifle_kills INTEGER DEFAULT 0,
			total_playtime_seconds INTEGER DEFAULT 0
		)
	`); err != nil {
		return fmt.Errorf("failed to create players table: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_players_kills ON players(total_kills DESC)`); err != nil {
		return fmt.Errorf("failed to create players kills index: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_players_last_seen ON players(last_seen DESC)`); err != nil {
		return fmt.Errorf("failed to create players last_seen index: %w", err)
	}

	// Sessions table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			session_id INTEGER PRIMARY KEY AUTOINCREMENT,
			player_id TEXT NOT NULL,
			room_id TEXT NOT NULL,
			map_id TEXT NOT NULL,
			start_time TIMESTAMP NOT NULL,
			end_time TIMESTAMP,
			duration_seconds INTEGER,
			kills INTEGER DEFAULT 0,
			deaths INTEGER DEFAULT 0,
			FOREIGN KEY (player_id) REFERENCES players(player_id)
		)
	`); err != nil {
		return fmt.Errorf("failed to create sessions table: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_player ON sessions(player_id)`); err != nil {
		return fmt.Errorf("failed to create sessions player index: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_room ON sessions(room_id)`); err != nil {
		return fmt.Errorf("failed to create sessions room index: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_start ON sessions(start_time DESC)`); err != nil {
		return fmt.Errorf("failed to create sessions start_time index: %w", err)
	}

	// Shots table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS shots (
			shot_id INTEGER PRIMARY KEY AUTOINCREMENT,
			player_id TEXT NOT NULL,
			room_id TEXT NOT NULL,
			weapon_type INTEGER NOT NULL,
			hit BOOLEAN NOT NULL,
			timestamp TIMESTAMP NOT NULL,
			FOREIGN KEY (player_id) REFERENCES players(player_id)
		)
	`); err != nil {
		return fmt.Errorf("failed to create shots table: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_shots_player ON shots(player_id)`); err != nil {
		return fmt.Errorf("failed to create shots player index: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_shots_timestamp ON shots(timestamp DESC)`); err != nil {
		return fmt.Errorf("failed to create shots timestamp index: %w", err)
	}

	// Kills table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS kills (
			kill_id INTEGER PRIMARY KEY AUTOINCREMENT,
			killer_id TEXT NOT NULL,
			victim_id TEXT NOT NULL,
			room_id TEXT NOT NULL,
			weapon_type INTEGER NOT NULL,
			timestamp TIMESTAMP NOT NULL,
			FOREIGN KEY (killer_id) REFERENCES players(player_id),
			FOREIGN KEY (victim_id) REFERENCES players(player_id)
		)
	`); err != nil {
		return fmt.Errorf("failed to create kills table: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_kills_killer ON kills(killer_id)`); err != nil {
		return fmt.Errorf("failed to create kills killer index: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_kills_victim ON kills(victim_id)`); err != nil {
		return fmt.Errorf("failed to create kills victim index: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_kills_timestamp ON kills(timestamp DESC)`); err != nil {
		return fmt.Errorf("failed to create kills timestamp index: %w", err)
	}

	// Rooms table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS rooms (
			room_id TEXT PRIMARY KEY,
			map_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			stopped_at TIMESTAMP,
			total_players INTEGER DEFAULT 0
		)
	`); err != nil {
		return fmt.Errorf("failed to create rooms table: %w", err)
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_rooms_created ON rooms(created_at DESC)`); err != nil {
		return fmt.Errorf("failed to create rooms created_at index: %w", err)
	}

	// Map stats table
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS map_stats (
			map_id TEXT PRIMARY KEY,
			games_played INTEGER DEFAULT 0,
			unique_players INTEGER DEFAULT 0,
			total_duration_seconds INTEGER DEFAULT 0,
			last_played TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("failed to create map_stats table: %w", err)
	}

	// Record schema version
	if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (1)"); err != nil {
		return fmt.Errorf("failed to record schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schema transaction: %w", err)
	}

	log.Println("statistics database schema initialized (version 1)")
	return nil
}
