package stats

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// API provides HTTP endpoints for querying statistics.
type API struct {
	db     *Database
	writer *Writer
	router *http.ServeMux
}

// NewAPI creates a new Statistics API with all endpoints registered.
func NewAPI(db *Database) (*API, error) {
	writer, err := NewWriter(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create writer: %w", err)
	}

	api := &API{
		db:     db,
		writer: writer,
		router: http.NewServeMux(),
	}

	// Register endpoints
	api.router.HandleFunc("/stats/player/", api.handleGetPlayer)
	api.router.HandleFunc("/stats/players/top", api.handleGetTopPlayers)
	api.router.HandleFunc("/stats/weapons", api.handleGetWeapons)
	api.router.HandleFunc("/stats/maps", api.handleGetMaps)
	api.router.HandleFunc("/stats/rooms", api.handleGetRooms)
	api.router.HandleFunc("/stats/events", api.handlePostEvents)
	api.router.HandleFunc("/stats/export/players", api.handleExportPlayers)
	api.router.HandleFunc("/stats/export/events", api.handleExportEvents)

	return api, nil
}

// ServeHTTP implements http.Handler interface.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

// Close closes the writer and releases resources.
func (a *API) Close() error {
	if a.writer != nil {
		return a.writer.Close()
	}
	return nil
}

// handleGetPlayer returns statistics for a single player.
// GET /stats/player/{player_id}
func (a *API) handleGetPlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Extract player_id from URL path
	playerID := strings.TrimPrefix(r.URL.Path, "/stats/player/")
	if playerID == "" || playerID == "/stats/player/" {
		http.Error(w, `{"error":"player_id required"}`, http.StatusBadRequest)
		return
	}

	stats, err := a.db.GetPlayerStats(playerID)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"player not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("stats API: failed to get player stats: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleGetTopPlayers returns top N players by kills.
// GET /stats/players/top?limit=10
func (a *API) handleGetTopPlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse limit parameter
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	players, err := a.db.GetTopPlayers(limit)
	if err != nil {
		log.Printf("stats API: failed to get top players: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(players)
}

// handleGetWeapons returns aggregated weapon statistics.
// GET /stats/weapons
func (a *API) handleGetWeapons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	weapons, err := a.db.GetWeaponStats()
	if err != nil {
		log.Printf("stats API: failed to get weapon stats: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(weapons)
}

// handleGetMaps returns map popularity statistics.
// GET /stats/maps
func (a *API) handleGetMaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	maps, err := a.db.GetMapStats()
	if err != nil {
		log.Printf("stats API: failed to get map stats: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(maps)
}

// handleGetRooms returns room statistics.
// GET /stats/rooms
func (a *API) handleGetRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	rooms, err := a.db.GetRoomStats()
	if err != nil {
		log.Printf("stats API: failed to get room stats: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

// handlePostEvents receives events from room containers.
// POST /stats/events
func (a *API) handlePostEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Decode JSON request body
	var payload struct {
		Events []json.RawMessage `json:"events"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Parse events
	var events []Event
	for _, raw := range payload.Events {
		var wrapper struct {
			Type string          `json:"Type"`
			Data json.RawMessage `json:"Data"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			log.Printf("stats API: failed to parse event wrapper: %v", err)
			continue
		}

		var event Event
		event.Type = wrapper.Type

		switch wrapper.Type {
		case "shot":
			var shot ShotEvent
			if err := json.Unmarshal(wrapper.Data, &shot); err != nil {
				log.Printf("stats API: failed to parse shot event: %v", err)
				continue
			}
			event.Data = shot

		case "kill":
			var kill KillEvent
			if err := json.Unmarshal(wrapper.Data, &kill); err != nil {
				log.Printf("stats API: failed to parse kill event: %v", err)
				continue
			}
			event.Data = kill

		case "session":
			var session SessionEvent
			if err := json.Unmarshal(wrapper.Data, &session); err != nil {
				log.Printf("stats API: failed to parse session event: %v", err)
				continue
			}
			event.Data = session

		case "room":
			var room RoomEvent
			if err := json.Unmarshal(wrapper.Data, &room); err != nil {
				log.Printf("stats API: failed to parse room event: %v", err)
				continue
			}
			event.Data = room

		default:
			log.Printf("stats API: unknown event type: %s", wrapper.Type)
			continue
		}

		events = append(events, event)
	}

	// Write events to database
	if err := a.writer.WriteBatch(events); err != nil {
		log.Printf("stats API: failed to write events: %v", err)
		http.Error(w, `{"error":"failed to write events"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// handleExportPlayers exports all players to CSV.
// GET /stats/export/players
func (a *API) handleExportPlayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	players, err := a.db.GetAllPlayers()
	if err != nil {
		log.Printf("stats API: failed to get all players: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Set CSV headers
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=\"players.csv\"")

	// Write CSV
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write header row
	csvWriter.Write([]string{
		"player_id", "first_seen", "last_seen", "total_kills", "total_deaths",
		"shots_fired", "shots_hit", "pistol_kills", "rifle_kills", "total_playtime_seconds",
	})

	// Write data rows
	for _, p := range players {
		csvWriter.Write([]string{
			p.PlayerID,
			p.FirstSeen.Format("2006-01-02 15:04:05"),
			p.LastSeen.Format("2006-01-02 15:04:05"),
			strconv.Itoa(p.TotalKills),
			strconv.Itoa(p.TotalDeaths),
			strconv.Itoa(p.ShotsFired),
			strconv.Itoa(p.ShotsHit),
			strconv.Itoa(p.PistolKills),
			strconv.Itoa(p.RifleKills),
			strconv.Itoa(p.TotalPlaytimeSeconds),
		})
	}
}

// handleExportEvents exports events within date range to JSON lines.
// GET /stats/export/events?start_date=2024-01-01&end_date=2024-12-31
func (a *API) handleExportEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse date range parameters
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	if startDate == "" || endDate == "" {
		http.Error(w, `{"error":"start_date and end_date required"}`, http.StatusBadRequest)
		return
	}

	events, err := a.db.GetEventsByDateRange(startDate, endDate)
	if err != nil {
		log.Printf("stats API: failed to get events: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Set JSON lines headers
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", "attachment; filename=\"events.jsonl\"")

	// Write JSON lines (one event per line)
	encoder := json.NewEncoder(w)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			log.Printf("stats API: failed to encode event: %v", err)
			break
		}
	}
}
