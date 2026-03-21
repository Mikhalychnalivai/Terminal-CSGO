package main

import (
	"log"
	"os"

	"hack2026mart/internal/game/room"
)

func main() {
	addr := getenv("ROOM_ADDR", ":7000")
	roomID := getenv("ROOM_ID", "arena")
	wadPath := getenv("WAD_PATH", "/assets/SHOOTER.WAD")
	mapName := getenv("WAD_MAP", "E1M2")

	srv, err := room.NewServer(addr, roomID, wadPath, mapName)
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
