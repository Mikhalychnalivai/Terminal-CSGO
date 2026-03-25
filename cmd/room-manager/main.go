package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type manager struct {
	roomImage string
	network   string
}

type roomRequest struct {
	RoomID string `json:"room_id"`
	// MapID: по умолчанию corridor5 (коридор, 5 комнат); см. ensureRoom.
	MapID string `json:"map_id"`
	// MapJSON: сырое тело JSON-карты (редактор SSH); только при map_id=custom|editor.
	MapJSON string `json:"map_json"`
}

type roomResponse struct {
	RoomID string `json:"room_id"`
	Addr   string `json:"addr"`
}

func main() {
	m := &manager{
		roomImage: getenv("ROOM_IMAGE", "doom-ssh-arena:latest"),
		network:   getenv("DOCKER_NETWORK", "hack2026mart_default"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/rooms/create", m.handleCreate)
	mux.HandleFunc("/rooms/get", m.handleGet)

	addr := getenv("MANAGER_ADDR", ":8080")
	log.Printf("room-manager listening at %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("room-manager failed: %v", err)
	}
}

func (m *manager) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeReq(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	addr, err := m.ensureRoom(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, roomResponse{RoomID: req.RoomID, Addr: addr})
}

func (m *manager) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeReq(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	addr, err := m.getRoomAddr(req.RoomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, roomResponse{RoomID: req.RoomID, Addr: addr})
}

func (m *manager) ensureRoom(req roomRequest) (string, error) {
	roomID := req.RoomID
	name := roomContainerName(roomID)
	exists, _, err := inspectContainer(name)
	if err != nil {
		return "", err
	}
	if exists {
		// "Create room" should always provide a fresh room from current image/version.
		if _, err := runDocker("rm", "-f", name); err != nil {
			return "", fmt.Errorf("remove old room: %w", err)
		}
	}

	tickMS := getenv("ROOM_TICK_MS", "4")
	spawnCount := getenv("SPAWN_COUNT", "10")
	spawnMinDist := getenv("SPAWN_MIN_DIST", "7")
	spawnSymmetry := getenv("SPAWN_SYMMETRY", "4")

	mapID := strings.ToLower(strings.TrimSpace(req.MapID))
	mapJSON := strings.TrimSpace(req.MapJSON)
	if mapID == "" {
		if mapJSON != "" {
			mapID = "custom"
		} else {
			mapID = getenv("DEFAULT_MAP_ID", "corridor5")
		}
	}

	args := []string{
		"create",
		"--name", name,
		"--network", m.network,
		"--restart", "unless-stopped",
		"-e", "ROOM_ADDR=:7000",
		"-e", "ROOM_ID=" + roomID,
		"-e", "ROOM_TICK_MS=" + tickMS,
	}
	if v := strings.TrimSpace(os.Getenv("ROOM_STATE_GZIP")); v != "" {
		args = append(args, "-e", "ROOM_STATE_GZIP="+v)
	}
	if v := strings.TrimSpace(os.Getenv("ROOM_GZIP_MIN_BYTES")); v != "" {
		args = append(args, "-e", "ROOM_GZIP_MIN_BYTES="+v)
	}
	args = append(args,
		"-e", "SPAWN_COUNT=" + spawnCount,
		"-e", "SPAWN_MIN_DIST=" + spawnMinDist,
		"-e", "SPAWN_SYMMETRY=" + spawnSymmetry,
	)

	switch mapID {
	case "custom", "editor":
		if mapJSON == "" {
			return "", fmt.Errorf("map_json required for custom map")
		}
		if len(mapJSON) > 2*1024*1024 {
			return "", fmt.Errorf("map_json too large (max 2 MiB)")
		}
		if !json.Valid([]byte(mapJSON)) {
			return "", fmt.Errorf("invalid map_json")
		}
		mapDir := getenv("ROOM_MAPS_DIR", "/data/maps")
		if err := os.MkdirAll(mapDir, 0755); err != nil {
			return "", fmt.Errorf("maps dir: %w", err)
		}
		fname := name + ".json"
		hostPath := filepath.Join(mapDir, fname)
		if err := os.WriteFile(hostPath, []byte(mapJSON), 0644); err != nil {
			return "", fmt.Errorf("write map: %w", err)
		}
		vol := getenv("ROOM_MAPS_DOCKER_VOLUME", "")
		if vol == "" {
			return "", fmt.Errorf("custom map needs ROOM_MAPS_DOCKER_VOLUME (docker volume name; see docker-compose)")
		}
		args = append(args,
			"-v", vol+":/data/maps:ro",
			"-e", "JSON_MAP_PATH=/data/maps/"+fname,
			"-e", "JSON_MAP_SCALE=1",
			"-e", "SPAWN_MODE=from_map",
		)
		args = appendJSONMapEnv(args)
		args = appendJSONScatterEnv(args)
	case "1", "corridor5", "corridor", "default", "map":
		args = append(args,
			"-e", "JSON_MAP_PATH=/assets/maps/corridor5.json",
			"-e", "SPAWN_MODE=from_map",
		)
		args = appendJSONMapEnv(args)
		args = appendJSONScatterEnv(args)
	default:
		return "", fmt.Errorf("unknown map_id: use 1, corridor5, custom (with map_json), …")
	}

	args = append(args, m.roomImage, "/usr/local/bin/room")

	_, err = runDocker(args...)
	if err != nil {
		return "", fmt.Errorf("create room: %w", err)
	}
	if _, err := runDocker("start", name); err != nil {
		return "", fmt.Errorf("start room: %w", err)
	}
	return name + ":7000", nil
}

func (m *manager) getRoomAddr(roomID string) (string, error) {
	name := roomContainerName(roomID)
	exists, running, err := inspectContainer(name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", errors.New("room does not exist")
	}
	if !running {
		return "", errors.New("room exists but is not running")
	}
	return name + ":7000", nil
}

func inspectContainer(name string) (bool, bool, error) {
	out, err := runDocker("inspect", "-f", "{{.State.Running}}", name)
	if err != nil {
		if strings.Contains(err.Error(), "No such object") {
			return false, false, nil
		}
		return false, false, err
	}
	return true, strings.TrimSpace(out) == "true", nil
}

func runDocker(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func roomContainerName(roomID string) string {
	id := strings.ToLower(strings.TrimSpace(roomID))
	if id == "" {
		id = "arena"
	}
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	id = re.ReplaceAllString(id, "-")
	return "room-" + id
}

func decodeReq(r *http.Request) (roomRequest, error) {
	var req roomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return roomRequest{}, errors.New("invalid json")
	}
	req.RoomID = strings.TrimSpace(req.RoomID)
	if req.RoomID == "" {
		req.RoomID = "arena"
	}
	req.MapID = strings.TrimSpace(req.MapID)
	req.MapJSON = strings.TrimSpace(req.MapJSON)
	return req, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(k, fallback string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	return v
}

// appendJSONMapEnv прокидывает опции разбора JSON (масштаб клетки, переворот Y) в room.
func appendJSONMapEnv(args []string) []string {
	if v := strings.TrimSpace(os.Getenv("JSON_MAP_FLIP_Y")); v != "" {
		args = append(args, "-e", "JSON_MAP_FLIP_Y="+v)
	}
	if v := strings.TrimSpace(os.Getenv("JSON_MAP_SCALE")); v != "" {
		args = append(args, "-e", "JSON_MAP_SCALE="+v)
	}
	if v := strings.TrimSpace(os.Getenv("JSON_SIMPLE_GRID")); v != "" {
		args = append(args, "-e", "JSON_SIMPLE_GRID="+v)
	}
	return args
}

// appendJSONScatterEnv: разброс спавнов на JSON-карте — только если явно задано.
// JSON_SPAWN_MODE=scatter или JSON_USE_SCATTER=1 у room-manager → JSON_USE_SCATTER=1 в room.
func appendJSONScatterEnv(args []string) []string {
	scatter := false
	if strings.EqualFold(strings.TrimSpace(os.Getenv("JSON_USE_SCATTER")), "1") {
		scatter = true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("JSON_SPAWN_MODE")), "scatter") {
		scatter = true
	}
	if scatter {
		args = append(args, "-e", "JSON_USE_SCATTER=1")
	}
	return args
}
