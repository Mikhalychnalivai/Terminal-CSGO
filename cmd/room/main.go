package main

import (
	"log"
	"os"
	"strings"

	"hack2026mart/internal/game/room"
)

func main() {
	addr := getenv("ROOM_ADDR", ":7000")
	roomID := getenv("ROOM_ID", "arena")
	jsonMapPath := strings.TrimSpace(os.Getenv("JSON_MAP_PATH"))
	wadPath := getenv("WAD_PATH", "")
	mapName := getenv("WAD_MAP", "E1M2")

	var srv *room.Server
	var err error
	if jsonMapPath != "" {
		log.Printf("room starting: JSON_MAP_PATH=%s", jsonMapPath)
		srv, err = room.NewServerFromJSON(addr, roomID, jsonMapPath)
	} else if strings.TrimSpace(wadPath) != "" {
		log.Printf("room starting: WAD map %s from %s", mapName, wadPath)
		srv, err = room.NewServer(addr, roomID, wadPath, mapName)
	} else {
		log.Fatal("set JSON_MAP_PATH to a .json map, or WAD_PATH + WAD_MAP for WAD")
	}
	if err != nil {
		log.Fatalf("room init failed: %v", err)
	}
	if err := srv.Run(); err != nil {
		log.Fatalf("room server failed: %v", err)
	}
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}
