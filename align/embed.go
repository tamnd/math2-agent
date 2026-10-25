package align

import (
	"hash/fnv"
	"math"
)

// An Embedder maps a text to a vector, so that texts with the same meaning have vectors that are near to each other. The aligner compares two vectors by their cosine.
//
// The aligner calls an Embedder on each item and on each candidate record. An Embedder must not send the text to a service that the operator did not approve.
type Embedder interface {
	// Name names the model and its version, for example hash/1. A proposal records it.
	Name() string
	// Embed returns the vector of the text. All the vectors of one Embedder have the same length.
	Embed(text string) []float32
}

// Hash is the default Embedder. It runs on the local machine and needs no model. It counts the words, the pairs of adjacent words and the groups of three letters of the text into a vector of Dim numbers, with a hash of each one as its place, and it scales the vector to length 1.
//
// Hash finds texts that use the same words. It does not find texts with the same meaning in different words, which is the work of a learned model.
type Hash struct {
	Dim int // the length of a vector. Zero means 1024.
}

// Name returns hash/1.
func (Hash) Name() string { return "hash/1" }

// Embed returns the vector of the text.
func (h Hash) Embed(text string) []float32 {
	dim := h.Dim
	if dim <= 0 {
		dim = 1024
	}
	v := make([]float32, dim)
	add := func(s string, w float32) {
		f := fnv.New64a()
		_, _ = f.Write([]byte(s))
		x := f.Sum64()
		sign := float32(1)
		if x>>63 == 1 {
			sign = -1
		}
		v[x%uint64(dim)] += sign * w // #nosec G115 -- dim is positive.
	}
	ws := words(text)
	for i, w := range ws {
		add("w:"+w, 1)
		if i > 0 {
			add("b:"+ws[i-1]+" "+w, 1)
		}
		r := []rune(" " + w + " ")
		for j := 0; j+3 <= len(r); j++ {
			add("c:"+string(r[j:j+3]), 0.5)
		}
	}
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	if n > 0 {
		s := float32(1 / math.Sqrt(n))
		for i := range v {
			v[i] *= s
		}
	}
	return v
}

// cosine returns the cosine of the angle between a and b, or 0 when one of them is zero or their lengths differ.
func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}
