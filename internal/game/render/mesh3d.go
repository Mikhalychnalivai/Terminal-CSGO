package render

import "math"

// Мир: X и Z — плоскость карты (как me.X / me.Y в 2D), Y — вверх. Камера смотрит вдоль +Z в camera space после worldToCamera.

type Vec3 struct {
	X, Y, Z float64
}

func (a Vec3) Sub(b Vec3) Vec3 { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Add(b Vec3) Vec3 { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Mul(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }

func dot3(a, b Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func cross3(a, b Vec3) Vec3 {
	return Vec3{
		a.Y*b.Z - a.Z*b.Y,
		a.Z*b.X - a.X*b.Z,
		a.X*b.Y - a.Y*b.X,
	}
}

// worldToCamera: yaw = me.Angle (как в 2D: forward = (cos, sin) в плоскости XZ).
// Базис: right = (-sin, cos), forward = (cos, sin) в XZ → в камере z_cam = расстояние вперёд.
func worldToCamera(p, camPos Vec3, yaw float64) Vec3 {
	d := p.Sub(camPos)
	c, s := math.Cos(yaw), math.Sin(yaw)
	return Vec3{
		X: -d.X*s + d.Z*c,
		Y: d.Y,
		Z: d.X*c + d.Z*s,
	}
}

// Vec4 для проекции (как XNA Vector4).
type Vec4 struct {
	X, Y, Z, W float64
}

// projMatrix — как Camera.ProjectionMatrix в Asciipocalypse (near, far, fov, aspect).
func projMatrix(near, far, fov, aspect float64) [16]float64 {
	nearScreenWidth := 2 * near * math.Tan(fov/2)
	nearScreenHeight := nearScreenWidth / aspect
	// Row-major M11..M44 как в C# Matrix constructor
	m11 := 2 * near / nearScreenWidth
	m22 := 2 * near / nearScreenHeight
	m33 := (far + near) / (far - near)
	m34 := 1.0
	m43 := -2 * near * far / (far - near)
	return [16]float64{
		m11, 0, 0, 0,
		0, m22, 0, 0,
		0, 0, m33, m34,
		0, 0, m43, 0,
	}
}

// mulMatVec4: как XNA Vector4.Transform — строка-вектор × матрица (столбцы M11..M41 для X).
func mulMatVec4(m [16]float64, p Vec4) Vec4 {
	return Vec4{
		X: m[0]*p.X + m[4]*p.Y + m[8]*p.Z + m[12]*p.W,
		Y: m[1]*p.X + m[5]*p.Y + m[9]*p.Z + m[13]*p.W,
		Z: m[2]*p.X + m[6]*p.Y + m[10]*p.Z + m[14]*p.W,
		W: m[3]*p.X + m[7]*p.Y + m[11]*p.Z + m[15]*p.W,
	}
}

func transformProj(v Vec3, proj [16]float64) Vec4 {
	return mulMatVec4(proj, Vec4{X: v.X, Y: v.Y, Z: v.Z, W: 1})
}

// barycentric — 2D в NDC (x,y); классическая формула через площади (устойчивее dot-Gram при «тонких» треугольниках).
func barycentric(p, v0, v1, v2 Vec3) Vec3 {
	px, py := p.X, p.Y
	ax, ay := v0.X, v0.Y
	bx, by := v1.X, v1.Y
	cx, cy := v2.X, v2.Y
	den := (by-cy)*(ax-cx) + (cx-bx)*(ay-cy)
	if math.Abs(den) < 1e-14 {
		return Vec3{X: 1, Y: 0, Z: 0}
	}
	w0 := ((by-cy)*(px-cx) + (cx-bx)*(py-cy)) / den
	w1 := ((cy-ay)*(px-cx) + (ax-cx)*(py-cy)) / den
	w2 := 1.0 - w0 - w1
	return Vec3{X: w0, Y: w1, Z: w2}
}
