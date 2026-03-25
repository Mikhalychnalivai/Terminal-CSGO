package protocol

import (
	"encoding/json"
	"math"
)

type ClientMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id,omitempty"`
	Name   string `json:"name,omitempty"`
	Key    string `json:"key,omitempty"`
	// При key=fire и key=r: 1 — пистолет, 2 — автомат (боезапас / перезарядка).
	Weapon int `json:"weapon"`
	// При key=buy: что купить — "ammo" или "armor".
	Buy string `json:"buy,omitempty"`
	// При key=ping: клиентский UnixNano для расчёта RTT (эхо в state.echo_ping_nano).
	PingNano int64 `json:"ping_nano,omitempty"`
	// PingRTTMs — последний измеренный RTT до room (мс), клиент шлёт для отображения другим игрокам.
	PingRTTMs int `json:"ping_rtt_ms,omitempty"`
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
	// KillFeed — последние убийства (сервер дублирует в каждом state; без omitempty — иначе пустой [] не шлётся).
	KillFeed []KillFeedEntry `json:"kill_feed"`
}

// UnmarshalJSON разбирает снимок так, чтобы каждый игрок проходил через UnmarshalJSON PlayerState
// (поля rifle_mag / rifle_reserve надёжно попадали в клиент при вложенном decode).
func (rs *RoomSnapshot) UnmarshalJSON(data []byte) error {
	var aux struct {
		RoomID      string            `json:"room_id"`
		Players     []json.RawMessage `json:"players"`
		Weapons     []WeaponSpawn     `json:"weapons"`
		Walls       []GridPoint       `json:"walls"`
		Width       int               `json:"width"`
		Height      int               `json:"height"`
		MapTitle    string            `json:"map_title"`
		WallTexture string            `json:"wall_texture"`
		CeilingFlat string            `json:"ceiling_flat"`
		FloorFlat   string            `json:"floor_flat"`
		KillFeed    []KillFeedEntry   `json:"kill_feed"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	rs.RoomID = aux.RoomID
	rs.Weapons = aux.Weapons
	rs.Walls = aux.Walls
	rs.Width = aux.Width
	rs.Height = aux.Height
	rs.MapTitle = aux.MapTitle
	rs.WallTexture = aux.WallTexture
	rs.CeilingFlat = aux.CeilingFlat
	rs.FloorFlat = aux.FloorFlat
	rs.KillFeed = aux.KillFeed
	if len(aux.Players) == 0 {
		rs.Players = nil
		return nil
	}
	rs.Players = make([]PlayerState, len(aux.Players))
	for i, raw := range aux.Players {
		if err := json.Unmarshal(raw, &rs.Players[i]); err != nil {
			return err
		}
	}
	return nil
}

// KillFeedEntry — одна строка килчата (кто убил кого).
type KillFeedEntry struct {
	Killer string `json:"killer"`
	Victim string `json:"victim"`
}

type PlayerState struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Angle float64 `json:"angle"`
	HP    int     `json:"hp"`
	Armor int     `json:"armor"`
	Dead  bool    `json:"dead,omitempty"`
	// Анимация спрайта (мир): Moving — недавний шаг; WalkPhase 0..7 — фаза шага (сервер).
	Moving    bool `json:"moving,omitempty"`
	WalkPhase int  `json:"walk_phase,omitempty"`
	// FireAgeMs — сколько миллисекунд прошло с последнего выстрела этого игрока (серверное время снапшота); 0 — нет вспышки.
	FireAgeMs int `json:"fire_age_ms,omitempty"`
	// HitConfirmAgeMs — возраст последнего попадания по врагу (для прицела «попал»).
	HitConfirmAgeMs int `json:"hit_confirm_age_ms,omitempty"`
	// KilledBy — отображается на экране смерти: имя убийцы (пока игрок мёртв).
	KilledBy string `json:"killed_by,omitempty"`
	// PistolMag — патроны в обойме пистолета (запас бесконечен, в JSON не передаётся).
	PistolMag int `json:"pistol_mag"`
	// RifleMag / RifleReserve — боезапас автомата.
	RifleMag     int `json:"rifle_mag"`
	RifleReserve int `json:"rifle_reserve"`
	// Money — игровая валюта (D).
	Money int `json:"money"`
	// Kills / Deaths — статистика за сессию.
	Kills  int `json:"kills"`
	Deaths int `json:"deaths"`
	// EchoPingNano — эхо последнего ping_nano клиента (один кадр; 0 = нет).
	EchoPingNano int64 `json:"echo_ping_nano,omitempty"`
	// PingMs — задержка до room (мс), сообщает сам клиент; для чужих игроков из снапшота.
	PingMs int `json:"ping_ms,omitempty"`
}

// MarshalJSON округляет angle и координаты (меньше байт в JSON, без потери для рендера).
func (p PlayerState) MarshalJSON() ([]byte, error) {
	type t struct {
		ID              string  `json:"id"`
		Name            string  `json:"name"`
		X               float64 `json:"x"`
		Y               float64 `json:"y"`
		Angle           float64 `json:"angle"`
		HP              int     `json:"hp"`
		Armor           int     `json:"armor"`
		Dead            bool    `json:"dead,omitempty"`
		Moving          bool    `json:"moving,omitempty"`
		WalkPhase       int     `json:"walk_phase,omitempty"`
		FireAgeMs       int     `json:"fire_age_ms,omitempty"`
		HitConfirmAgeMs int     `json:"hit_confirm_age_ms,omitempty"`
		KilledBy        string  `json:"killed_by,omitempty"`
		PistolMag       int     `json:"pistol_mag"`
		RifleMag        int     `json:"rifle_mag"`
		RifleReserve    int     `json:"rifle_reserve"`
		Money           int     `json:"money"`
		Kills           int     `json:"kills"`
		Deaths          int     `json:"deaths"`
		EchoPingNano    int64   `json:"echo_ping_nano,omitempty"`
		PingMs          int     `json:"ping_ms,omitempty"`
	}
	a := math.Round(p.Angle*10000) / 10000
	x := math.Round(p.X*10000) / 10000
	y := math.Round(p.Y*10000) / 10000
	return json.Marshal(t{
		ID: p.ID, Name: p.Name, X: x, Y: y, Angle: a, HP: p.HP, Armor: p.Armor, Dead: p.Dead,
		Moving: p.Moving, WalkPhase: p.WalkPhase, FireAgeMs: p.FireAgeMs, HitConfirmAgeMs: p.HitConfirmAgeMs,
		KilledBy: p.KilledBy, PistolMag: p.PistolMag, RifleMag: p.RifleMag, RifleReserve: p.RifleReserve,
		Money: p.Money, Kills: p.Kills, Deaths: p.Deaths, EchoPingNano: p.EchoPingNano,
		PingMs: p.PingMs,
	})
}

// UnmarshalJSON явно читает fire_age_ms (на случай расхождений с тегами при decode).
func (p *PlayerState) UnmarshalJSON(data []byte) error {
	var aux struct {
		ID              string  `json:"id"`
		Name            string  `json:"name"`
		X               float64 `json:"x"`
		Y               float64 `json:"y"`
		Angle           float64 `json:"angle"`
		HP              int     `json:"hp"`
		Armor           int     `json:"armor"`
		Dead            bool    `json:"dead"`
		Moving          bool    `json:"moving"`
		WalkPhase       int     `json:"walk_phase"`
		FireAgeMs       int     `json:"fire_age_ms"`
		HitConfirmAgeMs int     `json:"hit_confirm_age_ms"`
		KilledBy        string  `json:"killed_by"`
		PistolMag       int     `json:"pistol_mag"`
		RifleMag        int     `json:"rifle_mag"`
		RifleReserve    int     `json:"rifle_reserve"`
		Money           int     `json:"money"`
		Kills           int     `json:"kills"`
		Deaths          int     `json:"deaths"`
		EchoPingNano    int64   `json:"echo_ping_nano"`
		PingMs          int     `json:"ping_ms"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	p.ID = aux.ID
	p.Name = aux.Name
	p.X = aux.X
	p.Y = aux.Y
	p.Angle = aux.Angle
	p.HP = aux.HP
	p.Armor = aux.Armor
	p.Dead = aux.Dead
	p.Moving = aux.Moving
	p.WalkPhase = aux.WalkPhase
	p.FireAgeMs = aux.FireAgeMs
	p.HitConfirmAgeMs = aux.HitConfirmAgeMs
	p.KilledBy = aux.KilledBy
	p.PistolMag = aux.PistolMag
	p.RifleMag = aux.RifleMag
	p.RifleReserve = aux.RifleReserve
	p.Money = aux.Money
	p.Kills = aux.Kills
	p.Deaths = aux.Deaths
	p.EchoPingNano = aux.EchoPingNano
	p.PingMs = aux.PingMs
	return nil
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
