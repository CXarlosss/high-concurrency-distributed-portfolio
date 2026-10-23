package game

import "math"

type Vec2 struct {
	X float64
	Y float64
}

func (v Vec2) Add(o Vec2) Vec2       { return Vec2{X: v.X + o.X, Y: v.Y + o.Y} }
func (v Vec2) Sub(o Vec2) Vec2       { return Vec2{X: v.X - o.X, Y: v.Y - o.Y} }
func (v Vec2) Mul(s float64) Vec2    { return Vec2{X: v.X * s, Y: v.Y * s} }
func (v Vec2) LengthSq() float64     { return v.X*v.X + v.Y*v.Y }
func (v Vec2) Length() float64       { return math.Sqrt(v.LengthSq()) }
func (v Vec2) Normalize() Vec2 {
	l := v.Length()
	if l == 0 {
		return Vec2{}
	}
	return Vec2{X: v.X / l, Y: v.Y / l}
}
