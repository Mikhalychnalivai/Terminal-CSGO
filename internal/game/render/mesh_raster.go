package render

import (
	"math"
)

const meshEyeHeight = 0.52

const meshZFar = 1e9 - 1

// meshFogDistance — дистанция, на которой туман почти полный (метры в мире).
const meshFogDistance = 52.0

// clipTriCameraNear — отсечение по z>=zMin в пространстве камеры (+Z вперёд). Убирает вырождение
// проекции, когда вершина уходит за глаз или слишком близко (типичные «дыры» стены под углом).
func clipTriCameraNear(v0, v1, v2 Vec3, zMin float64) [][3]Vec3 {
	isectEdge := func(pOut, pIn Vec3) (Vec3, bool) {
		dz := pIn.Z - pOut.Z
		if math.Abs(dz) < 1e-9 {
			return Vec3{}, false
		}
		t := (zMin - pOut.Z) / dz
		if t < 0 || t > 1 {
			return Vec3{}, false
		}
		return Vec3{
			X: pOut.X + t*(pIn.X-pOut.X),
			Y: pOut.Y + t*(pIn.Y-pOut.Y),
			Z: zMin,
		}, true
	}
	v := [3]Vec3{v0, v1, v2}
	in := [3]bool{v[0].Z >= zMin, v[1].Z >= zMin, v[2].Z >= zMin}
	n := 0
	for _, b := range in {
		if b {
			n++
		}
	}
	switch n {
	case 0:
		return nil
	case 3:
		return [][3]Vec3{{v0, v1, v2}}
	case 1:
		var ii int
		for i := 0; i < 3; i++ {
			if in[i] {
				ii = i
				break
			}
		}
		a := v[ii]
		b := v[(ii+1)%3]
		c := v[(ii+2)%3]
		ib, ok1 := isectEdge(b, a)
		ic, ok2 := isectEdge(c, a)
		if !ok1 || !ok2 {
			return nil
		}
		return [][3]Vec3{{a, ib, ic}}
	case 2:
		var io int
		for i := 0; i < 3; i++ {
			if !in[i] {
				io = i
				break
			}
		}
		out := v[io]
		a := v[(io+1)%3]
		b := v[(io+2)%3]
		ica, ok1 := isectEdge(out, a)
		icb, ok2 := isectEdge(out, b)
		if !ok1 || !ok2 {
			return nil
		}
		return [][3]Vec3{
			{a, b, icb},
			{a, icb, ica},
		}
	default:
		return nil
	}
}

// rasterMeshScene — софт-растер треугольников в буфер терминала (по мотивам Asciipocalypse Rasterizer).
// Контуры: рёбра треугольников (барицентр), разрыв глубины, стык пол/стена/потолок; сетка на полу/потолке, «кирпичи» на стенах.
// colDepth — опционально длины viewW: по каждой колонке минимальная «исправленная» глубина до стены/пола (как у raycast) для окклюзии спрайтов; иначе nil.
func rasterMeshScene(scene [][]rune, colors [][]uint32, tris []MeshTri, camPos Vec3, yaw float64, fov float64, viewW, viewH int, colDepth []float64) {
	if len(tris) == 0 || viewW < 2 || viewH < 2 {
		return
	}
	aspect := float64(viewW) / float64(viewH)
	near, far := 0.08, 120.0
	clipZ := near + 1e-3
	proj := projMatrix(near, far, fov, aspect)

	zb := make([][]float64, viewH)
	kindBuf := make([][]uint8, viewH)
	for j := 0; j < viewH; j++ {
		zb[j] = make([]float64, viewW)
		kindBuf[j] = make([]uint8, viewW)
		for i := 0; i < viewW; i++ {
			zb[j][i] = meshZFar
			kindBuf[j][i] = 255
		}
	}

	for _, tri := range tris {
		cv0 := worldToCamera(tri.V0, camPos, yaw)
		cv1 := worldToCamera(tri.V1, camPos, yaw)
		cv2 := worldToCamera(tri.V2, camPos, yaw)
		clips := clipTriCameraNear(cv0, cv1, cv2, clipZ)
		if len(clips) == 0 {
			continue
		}
		for _, clip := range clips {
			v0, v1, v2 := clip[0], clip[1], clip[2]
			n := cross3(v1.Sub(v0), v2.Sub(v0))
			if dot3(n, v0) >= 0 {
				continue
			}

			p0 := transformProj(v0, proj)
			p1 := transformProj(v1, proj)
			p2 := transformProj(v2, proj)
			if p0.W == 0 || p1.W == 0 || p2.W == 0 {
				continue
			}

			ndc0 := Vec3{X: p0.X / p0.W, Y: -p0.Y / p0.W, Z: p0.Z / p0.W}
			ndc1 := Vec3{X: p1.X / p1.W, Y: -p1.Y / p1.W, Z: p1.Z / p1.W}
			ndc2 := Vec3{X: p2.X / p2.W, Y: -p2.Y / p2.W, Z: p2.Z / p2.W}

			// Для глубины и wp нужны clip Z и W; линейная смесь z_ndc по экрану даёт артефакты, зависящие от угла (стены «едут» при повороте).
			zc0, wc0 := p0.Z, p0.W
			zc1, wc1 := p1.Z, p1.W
			zc2, wc2 := p2.Z, p2.W

			minX := math.Min(ndc0.X, math.Min(ndc1.X, ndc2.X))
			maxX := math.Max(ndc0.X, math.Max(ndc1.X, ndc2.X))
			minY := math.Min(ndc0.Y, math.Min(ndc1.Y, ndc2.Y))
			maxY := math.Max(ndc0.Y, math.Max(ndc1.Y, ndc2.Y))

			minI := int(math.Max(0, math.Floor((minX+1)*0.5*float64(viewW))))
			maxI := int(math.Min(float64(viewW), math.Ceil((maxX+1)*0.5*float64(viewW))))
			minJ := int(math.Max(0, math.Floor((minY+1)*0.5*float64(viewH))))
			maxJ := int(math.Min(float64(viewH), math.Ceil((maxY+1)*0.5*float64(viewH))))

			if maxI <= minI || maxJ <= minJ {
				continue
			}

			// Барицентрики в NDC по центру пикселя (линейны по экрану).
			for i := minI; i < maxI; i++ {
				ndcX := (2*float64(i) + 1) / float64(viewW) - 1.0
				for j := minJ; j < maxJ; j++ {
					ndcY := (2*float64(j) + 1) / float64(viewH) - 1.0
					bar := barycentric(Vec3{X: ndcX, Y: ndcY, Z: 0}, ndc0, ndc1, ndc2)
					if bar.X < -1e-5 || bar.Y < -1e-5 || bar.Z < -1e-5 {
						continue
					}
					w0, w1, w2 := wc0, wc1, wc2
					if w0 < 1e-5 {
						w0 = 1e-5
					}
					if w1 < 1e-5 {
						w1 = 1e-5
					}
					if w2 < 1e-5 {
						w2 = 1e-5
					}
					denW := bar.X/w0 + bar.Y/w1 + bar.Z/w2
					if denW <= 1e-15 {
						continue
					}
					invDen := 1.0 / denW
					// Перспективно-корректный NDC z: Σ b_i·Z_clip_i/w_i² / Σ b_i/w_i
					z := (bar.X*zc0/(w0*w0) + bar.Y*zc1/(w1*w1) + bar.Z*zc2/(w2*w2)) * invDen
					// Чуть отодвигаем пол/потолок по глубине, чтобы на ребре со стеной не было z-fight и мерцания «кирпича».
					switch tri.Kind {
					case meshFloor:
						z += 3e-5
					case meshCeiling:
						z += 1.5e-5
					}
					if z > -1 && z < 1 && z < zb[j][i] {
						zb[j][i] = z
						kindBuf[j][i] = uint8(tri.Kind)

						wp := Vec3{
							X: (bar.X*tri.V0.X/w0 + bar.Y*tri.V1.X/w1 + bar.Z*tri.V2.X/w2) * invDen,
							Y: (bar.X*tri.V0.Y/w0 + bar.Y*tri.V1.Y/w1 + bar.Z*tri.V2.Y/w2) * invDen,
							Z: (bar.X*tri.V0.Z/w0 + bar.Y*tri.V1.Z/w1 + bar.Z*tri.V2.Z/w2) * invDen,
						}
						if len(colDepth) == viewW && (tri.Kind == meshWall || tri.Kind == meshFloor) {
							rayAng := yaw - fov/2 + fov*(float64(i)/float64(max(1, viewW-1)))
							t := (wp.X-camPos.X)*math.Cos(rayAng) + (wp.Z-camPos.Z)*math.Sin(rayAng)
							if t > 0.001 {
								corr := t * math.Cos(rayAng-yaw)
								if corr < 0.001 {
									corr = 0.001
								}
								if corr < colDepth[i] {
									colDepth[i] = corr
								}
							}
						}
						dx := wp.X - camPos.X
						dy := wp.Y - camPos.Y
						dz := wp.Z - camPos.Z
						dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
						fogT := dist / meshFogDistance
						if fogT > 1 {
							fogT = 1
						}
						// Плавнее середина дистанции, чем у NDC^8 — проще отличать «близко / далеко».
						fogT = fogT * fogT * (3.0 - 2.0*fogT)

						lum := int((1.0 - fogT) * 255)
						if tri.Kind == meshFloor {
							lum = int(float64(lum) * 0.68)
						}
						if tri.Kind == meshCeiling {
							lum = int(float64(lum) * 0.44)
						}

						lumF := float64(lum)

						switch tri.Kind {
						case meshFloor:
							// Шахматка клеток + сетка толще у ног (как подсказка перспективы).
							if (int(math.Floor(wp.X))+int(math.Floor(wp.Z)))&1 == 0 {
								lumF *= 1.08
							} else {
								lumF *= 0.90
							}
							margin := 0.028 + (1.0-fogT)*0.052
							if gridLine(wp.X, wp.Z, margin) {
								lumF *= 0.52
							}
						case meshCeiling:
							if gridLine(wp.X, wp.Z, 0.05) {
								lumF *= 0.60
							}
						case meshWall:
							lumF *= wallBrickPattern(wp, tri.WallAxis, dist)
							lumF *= wallHeightBands(wp, dist)
						}

						// Рёбра треугольника (тонкий контур).
						minBar := min3(bar.X, bar.Y, bar.Z)
						edgeTri := minBar < 0.04
						if edgeTri {
							lumF *= 0.68
						}

						lum = int(lumF)
						if lum < 0 {
							lum = 0
						}
						if lum > 255 {
							lum = 255
						}

						ch := depthGlyphFromLumJitter(lum, i, j)
						if edgeTri {
							ch = meshTriEdgeGlyph(i, j, tri.Kind)
						}
						scene[j][i] = ch
						colors[j][i] = meshShadeColor(&tri, fogT, i, j, wp)
					}
				}
			}
		}
	}

	meshPostProcessSilhouette(scene, colors, zb, kindBuf, viewW, viewH)
}

func min3(a, b, c float64) float64 {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// gridLine — линии сетки на клетках 1×1 в плоскости XZ.
func gridLine(x, z, margin float64) bool {
	fx := x - math.Floor(x)
	fz := z - math.Floor(z)
	return fx < margin || fx > 1.0-margin || fz < margin || fz > 1.0-margin
}

// wallDistBrickScale — ближе к стене выше частота кирпича в мировых координатах: в упоре и вдоль грани
// один «ряд» не растягивается на пол экрана (процедурная текстура остаётся читаемой).
func wallDistBrickScale(dist float64) float64 {
	d := math.Max(dist, 0.07)
	s := 0.42 / d
	if s < 1.0 {
		s = 1.0
	}
	if s > 5.5 {
		s = 5.5
	}
	return s
}

// wallBrickPattern — мельче «кирпич» + швы: проще читать высоту и расстояние, стена визуально легче.
func wallBrickPattern(wp Vec3, wallAxis uint8, dist float64) float64 {
	s := wallDistBrickScale(dist)
	sV := math.Min(4.0, math.Max(1.0, 0.32/math.Max(dist, 0.07)))
	m := 1.0
	fy := fract(wp.Y * 9.0 * sV)
	if fy < 0.065 || fy > 0.935 {
		m *= 0.82
	}
	switch wallAxis {
	case 1:
		fz := fract(wp.Z * s)
		if fz < 0.038 || fz > 0.962 {
			m *= 0.85
		}
	case 2:
		fx := fract(wp.X * s)
		if fx < 0.038 || fx > 0.962 {
			m *= 0.85
		}
	}
	return m
}

// wallHeightBands — доп. тонкие пояса по Y для глубины (не «монолит»).
func wallHeightBands(wp Vec3, dist float64) float64 {
	sV := math.Min(4.0, math.Max(1.0, 0.32/math.Max(dist, 0.07)))
	f := fract(wp.Y * 14.0 * sV)
	m := 1.0
	if f < 0.028 || f > 0.972 {
		m *= 0.93
	}
	return m
}

func meshTriEdgeGlyph(col, row int, kind meshTriKind) rune {
	switch kind {
	case meshFloor:
		return []rune{'+', '-', '*', '`'}[(col+row)&3]
	case meshCeiling:
		return []rune{':', '~', '.', ','}[(col+row)&3]
	default:
		return []rune{'|', '#', '/', ':'}[(col+row)&3]
	}
}

// meshPostProcessSilhouette — контур у границы с «пустотой» и на стыке пол / стена / потолок (без сравнения z на одной плоскости — иначе шум).
func meshPostProcessSilhouette(scene [][]rune, colors [][]uint32, zb [][]float64, kindBuf [][]uint8, viewW, viewH int) {
	for j := 0; j < viewH; j++ {
		for i := 0; i < viewW; i++ {
			z := zb[j][i]
			if z >= meshZFar-1e-6 {
				continue
			}
			k := kindBuf[j][i]
			if k == 255 {
				continue
			}
			sil := false
			neighbors := [4][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
			for _, d := range neighbors {
				ni, nj := i+d[0], j+d[1]
				if ni < 0 || ni >= viewW || nj < 0 || nj >= viewH {
					continue
				}
				zn := zb[nj][ni]
				kn := kindBuf[nj][ni]
				if zn >= meshZFar-1e-6 {
					sil = true
					break
				}
				if kn != k && kn != 255 {
					sil = true
					break
				}
			}
			if !sil {
				continue
			}
			col := uint32(colors[j][i])
			col = darkenPacked(col, 0.58)
			colors[j][i] = col
			scene[j][i] = meshSilhouetteGlyph(i, j, k)
		}
	}
}

func meshSilhouetteGlyph(i, j int, kindByte uint8) rune {
	kind := meshTriKind(kindByte)
	switch kind {
	case meshFloor:
		return []rune{'+', '=', '+', '-'}[(i+j)&3]
	case meshCeiling:
		return []rune{'~', '^', '~', '.'}[(i+j)&3]
	default:
		return []rune{'#', '|', '+', ':'}[(i+j)&3]
	}
}

// floorTileRGB — цвет плитки по клетке сущности пола (розово-кремовая шахматка + лёгкий сдвиг по координатам).
func floorTileRGB(gx, gz int, tNear float64) (r, g, b int) {
	t := tNear
	alt := (gx + gz) & 1
	if alt == 0 {
		r = 205 + int(t*48)
		g = 95 + int(t*42)
		b = 148 + int(t*40)
	} else {
		r = 178 + int(t*44)
		g = 78 + int(t*38)
		b = 128 + int(t*36)
	}
	r += (gx * 3) % 9
	g += (gz * 3) % 7
	b += ((gx + gz) * 2) % 8
	return r, g, b
}

func meshShadeColor(tri *MeshTri, fogT float64, i, j int, wp Vec3) uint32 {
	// fogT 0 близко, 1 далеко (после smoothstep от дистанции).
	tNear := 1.0 - fogT
	switch tri.Kind {
	case meshFloor:
		// Пол как сущность: у треугольника задана клетка FloorCell* (только проходимые клетки в меш-геометрии).
		if tri.FloorCellX >= 0 && tri.FloorCellZ >= 0 {
			r, g, b := floorTileRGB(int(tri.FloorCellX), int(tri.FloorCellZ), tNear)
			return RGBPacked(byte(clamp(r, 0, 255)), byte(clamp(g, 0, 255)), byte(clamp(b, 0, 255)))
		}
		t := tNear
		r := clamp(int(38+t*42), 0, 255)
		g := clamp(int(34+t*38), 0, 255)
		b := clamp(int(40+t*36), 0, 255)
		return RGBPacked(byte(r), byte(g), byte(b))
	case meshCeiling:
		t := tNear
		r := clamp(int(8+t*26), 0, 255)
		g := clamp(int(10+t*28), 0, 255)
		b := clamp(int(16+t*40), 0, 255)
		return RGBPacked(byte(r), byte(g), byte(b))
	default: // wall — светлее база, сильнее градиент по дистанции (читаемость размера/дальности).
		wallAxis := tri.WallAxis
		r := clamp(int(78-int(fogT*82)+int(tNear*8)), 18, 132)
		g := clamp(int(76-int(fogT*78)+int(tNear*6)), 16, 124)
		b := clamp(int(74-int(fogT*74)), 14, 112)
		switch wallAxis {
		case 1: // плоскость X — янтарнее (видишь «бок» вдоль Z)
			r = clamp(r+int(tNear*22), 0, 255)
			g = clamp(g+int(tNear*8), 0, 255)
			b = clamp(b-int(tNear*14), 0, 255)
		case 2: // плоскость Z — сине-зеленее (другой угол)
			r = clamp(r-int(tNear*12), 0, 255)
			g = clamp(g+int(tNear*18), 0, 255)
			b = clamp(b+int(tNear*24), 0, 255)
		}
		return RGBPacked(byte(r), byte(g), byte(b))
	}
}
