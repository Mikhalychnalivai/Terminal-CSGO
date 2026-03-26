package stats

import (
	"database/sql"
	"fmt"
)

// GetPlayerStats retrieves aggregated statistics for a single player.
func (db *Database) GetPlayerStats(playerID string) (*PlayerStats, error) {
	var stats PlayerStats

	err := db.conn.QueryRow(`
		SELECT player_id, total_kills, total_deaths, shots_fired, shots_hit,
		       pistol_kills, rifle_kills, total_playtime_seconds, last_seen
		FROM players
		WHERE player_id = ?
	`, playerID).Scan(
		&stats.PlayerID, &stats.TotalKills, &stats.TotalDeaths,
		&stats.ShotsFired, &stats.ShotsHit, &stats.PistolKills,
		&stats.RifleKills, &stats.TotalPlaytime, &stats.LastSeen,
	)

	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query player stats: %w", err)
	}

	// Calculate accuracy
	if stats.ShotsFired > 0 {
		stats.Accuracy = float64(stats.ShotsHit) / float64(stats.ShotsFired)
	}

	return &stats, nil
}

// GetTopPlayers retrieves top N players by total kills.
func (db *Database) GetTopPlayers(limit int) ([]PlayerStats, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := db.conn.Query(`
		SELECT player_id, total_kills, total_deaths, shots_fired, shots_hit,
		       pistol_kills, rifle_kills, total_playtime_seconds, last_seen
		FROM players
		ORDER BY total_kills DESC, total_deaths ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top players: %w", err)
	}
	defer rows.Close()

	var players []PlayerStats
	for rows.Next() {
		var p PlayerStats
		err := rows.Scan(
			&p.PlayerID, &p.TotalKills, &p.TotalDeaths,
			&p.ShotsFired, &p.ShotsHit, &p.PistolKills,
			&p.RifleKills, &p.TotalPlaytime, &p.LastSeen,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan player row: %w", err)
		}

		// Calculate accuracy
		if p.ShotsFired > 0 {
			p.Accuracy = float64(p.ShotsHit) / float64(p.ShotsFired)
		}

		players = append(players, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating player rows: %w", err)
	}

	return players, nil
}

// GetWeaponStats retrieves aggregated statistics for all weapon types.
func (db *Database) GetWeaponStats() ([]WeaponStats, error) {
	rows, err := db.conn.Query(`
		SELECT 
			weapon_type,
			COUNT(*) as total_shots,
			SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) as total_hits
		FROM shots
		GROUP BY weapon_type
		ORDER BY weapon_type
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query weapon stats: %w", err)
	}
	defer rows.Close()

	var weapons []WeaponStats
	for rows.Next() {
		var w WeaponStats
		err := rows.Scan(&w.WeaponType, &w.TotalShots, &w.TotalHits)
		if err != nil {
			return nil, fmt.Errorf("failed to scan weapon row: %w", err)
		}

		// Set weapon name
		if w.WeaponType == 1 {
			w.WeaponName = "Pistol"
		} else if w.WeaponType == 2 {
			w.WeaponName = "Rifle"
		} else {
			w.WeaponName = fmt.Sprintf("Weapon %d", w.WeaponType)
		}

		// Calculate accuracy
		if w.TotalShots > 0 {
			w.Accuracy = float64(w.TotalHits) / float64(w.TotalShots)
		}

		weapons = append(weapons, w)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating weapon rows: %w", err)
	}

	// Get kills per weapon
	for i := range weapons {
		var kills int
		err := db.conn.QueryRow(`
			SELECT COUNT(*) FROM kills WHERE weapon_type = ?
		`, weapons[i].WeaponType).Scan(&kills)
		if err != nil {
			return nil, fmt.Errorf("failed to query weapon kills: %w", err)
		}
		weapons[i].TotalKills = kills
	}

	return weapons, nil
}

// GetMapStats retrieves aggregated statistics for all maps, sorted by popularity.
func (db *Database) GetMapStats() ([]MapStatsResponse, error) {
	rows, err := db.conn.Query(`
		SELECT 
			r.map_id,
			COUNT(DISTINCT r.room_id) as games_played,
			COUNT(DISTINCT s.player_id) as unique_players,
			COALESCE(AVG(s.duration_seconds), 0) as avg_duration
		FROM rooms r
		LEFT JOIN sessions s ON r.room_id = s.room_id
		GROUP BY r.map_id
		ORDER BY games_played DESC, unique_players DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query map stats: %w", err)
	}
	defer rows.Close()

	var maps []MapStatsResponse
	for rows.Next() {
		var m MapStatsResponse
		var avgDuration float64
		err := rows.Scan(&m.MapID, &m.GamesPlayed, &m.UniquePlayers, &avgDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to scan map row: %w", err)
		}

		m.AvgDuration = int(avgDuration)
		maps = append(maps, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating map rows: %w", err)
	}

	return maps, nil
}

// GetRoomStats retrieves statistics for all rooms (active and historical).
func (db *Database) GetRoomStats() ([]Room, error) {
	rows, err := db.conn.Query(`
		SELECT room_id, map_id, created_at, stopped_at, total_players
		FROM rooms
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query room stats: %w", err)
	}
	defer rows.Close()

	var rooms []Room
	for rows.Next() {
		var r Room
		err := rows.Scan(&r.RoomID, &r.MapID, &r.CreatedAt, &r.StoppedAt, &r.TotalPlayers)
		if err != nil {
			return nil, fmt.Errorf("failed to scan room row: %w", err)
		}
		rooms = append(rooms, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating room rows: %w", err)
	}

	return rooms, nil
}

// GetAllPlayers retrieves all players for export (no limit).
func (db *Database) GetAllPlayers() ([]Player, error) {
	rows, err := db.conn.Query(`
		SELECT player_id, first_seen, last_seen, total_kills, total_deaths,
		       shots_fired, shots_hit, pistol_kills, rifle_kills, total_playtime_seconds
		FROM players
		ORDER BY total_kills DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query all players: %w", err)
	}
	defer rows.Close()

	var players []Player
	for rows.Next() {
		var p Player
		err := rows.Scan(
			&p.PlayerID, &p.FirstSeen, &p.LastSeen, &p.TotalKills, &p.TotalDeaths,
			&p.ShotsFired, &p.ShotsHit, &p.PistolKills, &p.RifleKills, &p.TotalPlaytimeSeconds,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan player row: %w", err)
		}
		players = append(players, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating player rows: %w", err)
	}

	return players, nil
}

// GetEventsByDateRange retrieves all events within a date range for export.
func (db *Database) GetEventsByDateRange(startDate, endDate string) ([]Event, error) {
	// Query shots
	shotRows, err := db.conn.Query(`
		SELECT player_id, room_id, weapon_type, hit, timestamp
		FROM shots
		WHERE timestamp BETWEEN ? AND ?
		ORDER BY timestamp
	`, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query shots: %w", err)
	}
	defer shotRows.Close()

	var events []Event
	for shotRows.Next() {
		var shot ShotEvent
		err := shotRows.Scan(&shot.PlayerID, &shot.RoomID, &shot.WeaponType, &shot.Hit, &shot.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan shot row: %w", err)
		}
		events = append(events, Event{Type: "shot", Data: shot})
	}

	// Query kills
	killRows, err := db.conn.Query(`
		SELECT killer_id, victim_id, room_id, weapon_type, timestamp
		FROM kills
		WHERE timestamp BETWEEN ? AND ?
		ORDER BY timestamp
	`, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query kills: %w", err)
	}
	defer killRows.Close()

	for killRows.Next() {
		var kill KillEvent
		err := killRows.Scan(&kill.KillerID, &kill.VictimID, &kill.RoomID, &kill.WeaponType, &kill.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan kill row: %w", err)
		}
		events = append(events, Event{Type: "kill", Data: kill})
	}

	return events, nil
}
