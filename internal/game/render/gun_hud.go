package render

import (
	"math"
)

// PlayerStatsData holds player statistics for display in overlay.
type PlayerStatsData struct {
	PlayerID      string
	TotalKills    int
	TotalDeaths   int
	ShotsFired    int
	ShotsHit      int
	Accuracy      float64
	PistolKills   int
	RifleKills    int
	TotalPlaytime int
	LastSeen      string
}

// Цвета пистолета на HUD (часть глифов в pistol_hud.go; см. также PistolHUDBrownBeigeBlend).
var (
	PistolHUDBrown = RGBPacked(140, 130, 125) // символ '#'
	PistolHUDBeige = RGBPacked(210, 210, 215) // корпус (_, \), не '#'
	PistolHUDFlash = RGBPacked(255, 240, 120)  // '*' на кадре огня
)

// PistolHUDBrownBeigeBlend смешивает бежевый и коричневый тон пистолета: step 0 — беж, 5 — коричневый
// (без затемнения; скобки автомата — те же оттенки, что и у палитры пистолета по шагам).
func PistolHUDBrownBeigeBlend(step int) uint32 {
	var r, g, b float64
	if step <= 0 {
		r, g, b = 210, 210, 215
	} else if step >= 5 {
		r, g, b = 140, 130, 125
	} else {
		t := float64(step) / 5.0
		r = 210 + (140-210)*t
		g = 210 + (130-210)*t
		b = 215 + (125-215)*t
	}
	const scale = 1.0 // в 2 раза светлее, чем при 1.5/3 (=0.5); полная яркость смеси пистолета
	rr := byte(math.Round(r * scale))
	gg := byte(math.Round(g * scale))
	bb := byte(math.Round(b * scale))
	return RGBPacked(rr, gg, bb)
}

// Выбор оружия на HUD (только клиент): 1 — пистолет, 2 — автомат (как в Doom).
const (
	HUDWeaponPistol = 1
	HUDWeaponRifle  = 2
	// HUDPistolDamage — урон на сервере при weapon=HUDWeaponPistol или по умолчанию.
	HUDPistolDamage = 20
	// HUDRifleMagMax — размер магазина автомата; патроны в запасе при спавне — HUDRifleReserveSpawn.
	HUDRifleMagMax       = 30
	HUDRifleReserveSpawn = 60
	// HUDPistolMagMax — патронов в обойме пистолета; запас бесконечен (сервер не хранит).
	HUDPistolMagMax = 10
)

// Экономика и магазин (B): стартовые деньги, награда за килл, цены и лимиты.
const (
	StartingMoney        = 100
	KillRewardMoney      = 100
	ShopAmmoPrice        = 50
	ShopAmmoRounds       = 30
	ShopMaxRifleReserve  = 120 // запас патронов автомата (покупка до этого потолка)
	ShopArmorPrice       = 100
	ShopArmorAdd         = 100
	ShopMaxArmor         = 100
)

// MoneyGainFlashNanos — длительность жёлтой вспышки «+N» над балансом при начислении денег (клиент).
const MoneyGainFlashNanos int64 = 1_000_000_000

// ReloadAnimTotalNanos — длительность анимации перезарядки автомата на HUD (клиент).
const ReloadAnimTotalNanos int64 = 1_000_000_000

// PistolReloadAnimTotalNanos — анимация перезарядки пистолета (короче автомата).
const PistolReloadAnimTotalNanos int64 = 750_000_000

// GunHUDState задаёт анимацию оружия по времени (как в Doom), а не по счётчику кадров SSH.
type GunHUDState struct {
	FireStartUnixNano int64 // 0 — нет анимации выстрела
	// ReloadStartUnixNano — начало анимации перезарядки (пистолет или автомат); 0 — нет.
	ReloadStartUnixNano int64
	NowUnixNano         int64
	Walking             bool // недавнее движение (W/S)
	// Weapon: 0 или HUDWeaponPistol (1) — пистолет, HUDWeaponRifle (2) — автомат.
	Weapon int
	// Поворот (клиент по дельте угла): последний заметный поворот и направление (-1 влево, +1 вправо).
	TurnLastUnixNano int64
	TurnDir          int
	// DamageFlashUntilUnixNano — красная кайма по краям, пока now < этого времени (клиент).
	DamageFlashUntilUnixNano int64
	// BuyMenuOpen — оверлей магазина (клиент, клавиша B).
	BuyMenuOpen bool
	// ScoreboardOpen — таблица игроков (Tab).
	ScoreboardOpen bool
	// StatsOverlayOpen — персональная статистика игрока (P).
	StatsOverlayOpen bool
	// CachedStats — кэшированная статистика для отображения.
	CachedStats *PlayerStatsData
	// PingRTTms — измеренный RTT до room (мс), по эхо ping в state.
	PingRTTMs int
	// StateLagMs — мс с последнего state на клиенте; в табе показывается как ~N, если RTT ещё не измерен.
	StateLagMs int
	// MoneyGainFlashUntilUnixNano / MoneyGainAmount — жёлтое мерцание «+N» над D при начислении (клиент).
	MoneyGainFlashUntilUnixNano int64
	MoneyGainAmount             int
}

// ~Doom: 35 тиков/сек; кадр выстрела каждые ~2 тика.
const (
	fireFrameNanos int64 = 57e6 // автомат и общая логика
	// pistolFireFrameNanos — дольше удержание кадра B/C/D, чтобы отдача на HUD читалась.
	pistolFireFrameNanos int64 = 82e6
	fireSeqLen           = 4
)

// pistolFrameFromHUD — индекс кадра B→C→D→A для автомата (быстрый шаг).
func pistolFrameFromHUD(hud GunHUDState, n int) int {
	return fireSequenceFrameIndex(hud, n, fireFrameNanos)
}

// pistolHUDFrameFromHUD — то же для пистолета, с более медленной серией выстрела.
func pistolHUDFrameFromHUD(hud GunHUDState, n int) int {
	return fireSequenceFrameIndex(hud, n, pistolFireFrameNanos)
}

func fireSequenceFrameIndex(hud GunHUDState, n int, frameNanos int64) int {
	if n <= 0 {
		return 0
	}
	if hud.FireStartUnixNano == 0 {
		return 0
	}
	elapsed := hud.NowUnixNano - hud.FireStartUnixNano
	if elapsed < 0 {
		return 0
	}
	totalFire := frameNanos * int64(fireSeqLen)
	if elapsed >= totalFire {
		return 0
	}
	step := int(elapsed / frameNanos)
	if step > fireSeqLen-1 {
		step = fireSeqLen - 1
	}
	seq := []int{1, 2, 3, 0}
	if step >= len(seq) {
		step = len(seq) - 1
	}
	f := seq[step]
	if f >= n {
		return n - 1
	}
	return f
}

// pistolFireRecoil — отдача пистолета: влево и вверх по экрану (dx<0, dy<0).
func pistolFireRecoil(fi int) (dx, dy int) {
	switch fi {
	case 1:
		return -5, -4
	case 2:
		return -8, -6
	case 3:
		return -4, -3
	default:
		return 0, 0
	}
}

// walkBobOffsets даёт смещение пистолета при ходьбе (строки вниз, колонки вправо).
func walkBobOffsets(hud GunHUDState) (dy, dx int) {
	if !hud.Walking {
		return 0, 0
	}
	t := float64(hud.NowUnixNano) / 1e9
	// Два цикла покачивания в секунду, как оружейный bob в Doom.
	dy = int(math.Round(math.Sin(t*2.0*math.Pi*2.0) * 1.8))
	dx = int(math.Round(math.Cos(t*2.0*math.Pi*2.0) * 0.9))
	return dy, dx
}

const turnSwayNanos = 180e6 // длительность качания после поворота

// turnSwayOffsets — инерция оружия при повороте (A/D): против направления поворота, затухание.
func turnSwayOffsets(hud GunHUDState) (dy, dx int) {
	if hud.TurnLastUnixNano == 0 || hud.TurnDir == 0 {
		return 0, 0
	}
	elapsed := hud.NowUnixNano - hud.TurnLastUnixNano
	if elapsed < 0 || elapsed > turnSwayNanos {
		return 0, 0
	}
	t := 1.0 - float64(elapsed)/turnSwayNanos
	// Поворот влево (dir=-1) — ствол «уходит» вправо: против знака TurnDir.
	sway := -float64(hud.TurnDir) * 2.8 * t
	dx = int(math.Round(sway))
	dy = int(math.Round(math.Sin(t*math.Pi) * -1.2))
	return dy, dx
}

// RifleReloadAnchorOffset — смещение автомата при перезарядке (опускание/подъём), elapsed с ReloadStart.
func RifleReloadAnchorOffset(elapsed int64) (dx, dy int) {
	if elapsed <= 0 || elapsed >= ReloadAnimTotalNanos {
		return 0, 0
	}
	t := float64(elapsed) / float64(ReloadAnimTotalNanos)
	dip := math.Sin(t*math.Pi) * 9.0
	dx = int(math.Round(math.Sin(t*math.Pi*4) * 2.2))
	dy = int(math.Round(dip))
	return dx, dy
}

func weaponNorm(w int) int {
	if w == 0 {
		return HUDWeaponPistol
	}
	return w
}

func rifleReloadAnimActive(hud GunHUDState) bool {
	if hud.ReloadStartUnixNano == 0 || weaponNorm(hud.Weapon) != HUDWeaponRifle {
		return false
	}
	el := hud.NowUnixNano - hud.ReloadStartUnixNano
	return el > 0 && el < ReloadAnimTotalNanos
}

func pistolReloadAnimActive(hud GunHUDState) bool {
	if hud.ReloadStartUnixNano == 0 || weaponNorm(hud.Weapon) != HUDWeaponPistol {
		return false
	}
	el := hud.NowUnixNano - hud.ReloadStartUnixNano
	return el > 0 && el < PistolReloadAnimTotalNanos
}

// PistolReloadAnchorOffset — смещение пистолета при перезарядке (легче, чем у автомата).
func PistolReloadAnchorOffset(elapsed int64) (dx, dy int) {
	if elapsed <= 0 || elapsed >= PistolReloadAnimTotalNanos {
		return 0, 0
	}
	t := float64(elapsed) / float64(PistolReloadAnimTotalNanos)
	dip := math.Sin(t*math.Pi) * 6.5
	dx = int(math.Round(math.Sin(t*math.Pi*3) * 1.8))
	dy = int(math.Round(dip))
	return dx, dy
}

// rifleFireRecoil — отдача ASCII-автомата по кадру выстрела (0 = idle).
func rifleFireRecoil(fi int) (dx, dy int) {
	switch fi {
	case 1:
		return -2, 1
	case 2:
		return -3, 2
	case 3:
		return -1, 1
	default:
		return 0, 0
	}
}

// showMuzzleFlash — вспышка в начале выстрела (у пистолета дольше — под pistolFireFrameNanos).
func showMuzzleFlash(hud GunHUDState) bool {
	if rifleReloadAnimActive(hud) || pistolReloadAnimActive(hud) {
		return false
	}
	if hud.FireStartUnixNano == 0 {
		return false
	}
	elapsed := hud.NowUnixNano - hud.FireStartUnixNano
	if elapsed < 0 {
		return false
	}
	w := hud.Weapon
	if w == 0 || w == HUDWeaponPistol {
		return elapsed < pistolFireFrameNanos+35e6
	}
	return elapsed < 70e6
}
