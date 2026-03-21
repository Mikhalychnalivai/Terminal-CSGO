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
	"regexp"
	"strings"
	"time"
)

type manager struct {
	roomImage string
	network   string
	wadMap    string
}

type roomRequest struct {
	RoomID string `json:"room_id"`
}

type roomResponse struct {
	RoomID string `json:"room_id"`
	Addr   string `json:"addr"`
}

func main() {
	m := &manager{
		roomImage: getenv("ROOM_IMAGE", "shooter-ssh-arena:latest"),
		network:   getenv("DOCKER_NETWORK", "hack2026mart_default"),
		wadMap:    getenv("WAD_MAP", "E1M2"),
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
	addr, err := m.ensureRoom(req.RoomID)
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

func (m *manager) ensureRoom(roomID string) (string, error) {
	name := roomContainerName(roomID)
	exists, _, err := inspectContainer(name)
	if err != nil {
		return "", err
	}
	if exists {
		if _, err := runDocker("rm", "-f", name); err != nil {
			return "", fmt.Errorf("remove old room: %w", err)
		}
	}

	tickMS := getenv("ROOM_TICK_MS", "16")
	spawnMode := getenv("SPAWN_MODE", "scatter")
	spawnCount := getenv("SPAWN_COUNT", "10")
	spawnMinDist := getenv("SPAWN_MIN_DIST", "7")
	spawnSymmetry := getenv("SPAWN_SYMMETRY", "4")
	_, err = runDocker(
		"create",
		"--name", name,
		"--network", m.network,
		"--restart", "unless-stopped",
		"-e", "ROOM_ADDR=:7000",
		"-e", "ROOM_ID="+roomID,
		"-e", "WAD_PATH=/assets/SHOOTER.WAD",
		"-e", "WAD_MAP="+m.wadMap,
		"-e", "ROOM_TICK_MS="+tickMS,
		"-e", "SPAWN_MODE="+spawnMode,
		"-e", "SPAWN_COUNT="+spawnCount,
		"-e", "SPAWN_MIN_DIST="+spawnMinDist,
		"-e", "SPAWN_SYMMETRY="+spawnSymmetry,
		m.roomImage,
		"/usr/local/bin/room",
	)
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
