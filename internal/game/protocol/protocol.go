package protocol

type ClientMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id,omitempty"`
	Name   string `json:"name,omitempty"`
	Key    string `json:"key,omitempty"`
}

type ServerMessage struct {
	Type      string        `json:"type"`
	PlayerID  string        `json:"player_id,omitempty"`
	RoomID    string        `json:"room_id,omitempty"`
	Width     int           `json:"width,omitempty"`
	Height    int           `json:"height,omitempty"`
	Error     string        `json:"error,omitempty"`
	State     *RoomSnapshot `json:"state,omitempty"`
	LobbyText string        `json:"lobby_text,omitempty"`
}

type RoomSnapshot struct {
	RoomID   string         `json:"room_id"`
	Players  []PlayerState  `json:"players"`
	Weapons  []WeaponSpawn  `json:"weapons,omitempty"` // пусто = клиент держит последний полный снапшот
	Walls    []GridPoint    `json:"walls,omitempty"`   // то же (экономия трафика ~сотни KB/тик)
	Width    int            `json:"width"`
	Height   int            `json:"height"`
	MapTitle string         `json:"map_title"`
	// Doom WAD texture names (from SIDEDEFS / SECTORS) for client-side sampling.
	WallTexture string `json:"wall_texture,omitempty"`
	CeilingFlat string `json:"ceiling_flat,omitempty"`
	FloorFlat   string `json:"floor_flat,omitempty"`
}

type PlayerState struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Angle float64 `json:"angle"`
}

type WeaponSpawn struct {
	Name string `json:"name"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

type GridPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}
