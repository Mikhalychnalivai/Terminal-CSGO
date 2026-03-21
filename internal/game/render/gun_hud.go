package render

import (
	"math"
)

// GunHUDState задаёт анимацию оружия по времени (как в shooter), а не по счётчику кадров SSH.
type GunHUDState struct {
	FireStartUnixNano int64
	NowUnixNano       int64
	Walking           bool
}

// ~Shooter: 35 тиков/сек; кадр выстрела каждые ~2 тика.
const (
	fireFrameNanos int64 = 57e6
	fireSeqLen           = 4
)

func pistolFrameFromHUD(hud GunHUDState, n int) int {
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
	totalFire := fireFrameNanos * int64(fireSeqLen)
	if elapsed >= totalFire {
		return 0
	}
	step := int(elapsed / fireFrameNanos)
	if step > fireSeqLen-1 {
		step = fireSeqLen - 1
	}
	// Как в ванильном shooter: B → C → D → откат к A в конце серии.
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

func walkBobOffsets(hud GunHUDState) (dy, dx int) {
	if !hud.Walking {
		return 0, 0
	}
	t := float64(hud.NowUnixNano) / 1e9
	// Два цикла покачивания в секунду, как оружейный bob в shooter.
	dy = int(math.Round(math.Sin(t*2.0*math.Pi*2.0) * 1.8))
	dx = int(math.Round(math.Cos(t*2.0*math.Pi*2.0) * 0.9))
	return dy, dx
}

func showTracer(hud GunHUDState) bool {
	if hud.FireStartUnixNano == 0 {
		return false
	}
	elapsed := hud.NowUnixNano - hud.FireStartUnixNano
	return elapsed >= 0 && elapsed < 130e6
}

func showMuzzleFlash(hud GunHUDState) bool {
	if hud.FireStartUnixNano == 0 {
		return false
	}
	elapsed := hud.NowUnixNano - hud.FireStartUnixNano
	return elapsed >= 0 && elapsed < 70e6
}
