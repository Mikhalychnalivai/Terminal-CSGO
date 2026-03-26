package stats

import "time"

// Player represents aggregated statistics for a single player.
type Player struct {
	PlayerID             string
	FirstSeen            time.Time
	LastSeen             time.Time
	TotalKills           int
	TotalDeaths          int
	ShotsFired           int
	ShotsHit             int
	PistolKills          int
	RifleKills           int
	TotalPlaytimeSeconds int
}

// Session represents a single player session in a room.
type Session struct {
	SessionID       int64
	PlayerID        string
	RoomID          string
	MapID           string
	StartTime       time.Time
	EndTime         *time.Time
	DurationSeconds *int
	Kills           int
	Deaths          int
}

// Shot represents a single weapon fire event.
type Shot struct {
	ShotID     int64
	PlayerID   string
	RoomID     string
	WeaponType int
	Hit        bool
	Timestamp  time.Time
}

// Kill represents a kill event (killer kills victim).
type Kill struct {
	KillID     int64
	KillerID   string
	VictimID   string
	RoomID     string
	WeaponType int
	Timestamp  time.Time
}

// Room represents a room lifecycle tracking.
type Room struct {
	RoomID       string
	MapID        string
	CreatedAt    time.Time
	StoppedAt    *time.Time
	TotalPlayers int
}

// MapStats represents aggregated statistics for a map.
type MapStats struct {
	MapID                string
	GamesPlayed          int
	UniquePlayers        int
	TotalDurationSeconds int
	LastPlayed           *time.Time
}

// PlayerStats is the API response format for player statistics.
type PlayerStats struct {
	PlayerID      string  `json:"player_id"`
	TotalKills    int     `json:"total_kills"`
	TotalDeaths   int     `json:"total_deaths"`
	ShotsFired    int     `json:"shots_fired"`
	ShotsHit      int     `json:"shots_hit"`
	Accuracy      float64 `json:"accuracy"`
	PistolKills   int     `json:"pistol_kills"`
	RifleKills    int     `json:"rifle_kills"`
	TotalPlaytime int     `json:"total_playtime_seconds"`
	LastSeen      string  `json:"last_seen"`
}

// WeaponStats is the API response format for weapon statistics.
type WeaponStats struct {
	WeaponType int     `json:"weapon_type"`
	WeaponName string  `json:"weapon_name"`
	TotalShots int     `json:"total_shots"`
	TotalHits  int     `json:"total_hits"`
	TotalKills int     `json:"total_kills"`
	Accuracy   float64 `json:"accuracy"`
}

// MapStatsResponse is the API response format for map statistics.
type MapStatsResponse struct {
	MapID         string `json:"map_id"`
	GamesPlayed   int    `json:"games_played"`
	UniquePlayers int    `json:"unique_players"`
	AvgDuration   int    `json:"avg_duration_seconds"`
}
