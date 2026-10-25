package search

import (
	"encoding/binary"
	"errors"
	"math"
)

// A vector is an embedding in int8, with the scale that gives back the float values (spec 14.4). The index holds only these, and the results never hold them.
type vector struct {
	scale float32
	v     []int8
}

func quantize(f []float32) vector {
	var m float32
	for _, x := range f {
		m = max(m, float32(math.Abs(float64(x))))
	}
	q := vector{v: make([]int8, len(f))}
	if m == 0 {
		return q
	}
	q.scale = m / 127
	for i, x := range f {
		q.v[i] = int8(math.Round(float64(x / q.scale))) // #nosec G115 -- |x / scale| is at most 127.
	}
	return q
}

// dot returns the dot product of two vectors of the same length, or 0 when the lengths differ. For vectors of length 1, it is the cosine.
func dot(a, b vector) float64 {
	if len(a.v) != len(b.v) {
		return 0
	}
	var s int64
	for i := range a.v {
		s += int64(a.v[i]) * int64(b.v[i])
	}
	return float64(s) * float64(a.scale) * float64(b.scale)
}

// The bytes of a vector are the scale as a float32 in little-endian order, then one byte for each number.
func (v vector) bytes() []byte {
	out := make([]byte, 4+len(v.v))
	binary.LittleEndian.PutUint32(out, math.Float32bits(v.scale))
	for i, x := range v.v {
		out[4+i] = byte(x) // #nosec G115 -- the byte is the bits of the int8.
	}
	return out
}

func readVector(b []byte) (vector, error) {
	if len(b) < 4 {
		return vector{}, errors.New("search: a vector has fewer than 4 bytes")
	}
	v := vector{scale: math.Float32frombits(binary.LittleEndian.Uint32(b)), v: make([]int8, len(b)-4)}
	for i, x := range b[4:] {
		v.v[i] = int8(x) // #nosec G115 -- the byte is the bits of the int8.
	}
	return v, nil
}
