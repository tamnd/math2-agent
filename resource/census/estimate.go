package census

import (
	"math"
	"math/rand/v2"
	"slices"
)

// Chapman gives the estimate of the size of the universe from two catalogues A and B that sample it independently (RS11.4.1). nA and nB are the works of the stratum in A and in B, and m is the works in both.
func Chapman(nA, nB, m int) float64 {
	return float64(nA+1)*float64(nB+1)/float64(m+1) - 1
}

// Recall gives n / est, the recall of a catalogue with n works of a stratum whose size is est. It gives 0 when est is not positive.
func Recall(n int, est float64) float64 {
	if est <= 0 {
		return 0
	}
	return float64(n) / est
}

// Interval is a confidence interval.
type Interval struct {
	Low, High float64
}

// Estimate is the Chapman estimate of one stratum for one pair of catalogues, with its interval.
type Estimate struct {
	NA, NB, M int
	N         float64  // the estimate of the size of the universe
	CI        Interval // the 95% interval, by the bootstrap over the works
}

// Bootstraps is the number of bootstrap samples of Pair.
const Bootstraps = 1000

// Pair gives the Chapman estimate of a stratum and its 95% interval by the bootstrap over the works (RS11.4.1). The bootstrap draws the works seen in A or in B with replacement, Bootstraps times, from a generator with the seed, so the same input gives the same interval.
func Pair(nA, nB, m int, seed uint64) Estimate {
	e := Estimate{NA: nA, NB: nB, M: m, N: Chapman(nA, nB, m)}
	onlyA, onlyB := nA-m, nB-m
	seen := onlyA + onlyB + m
	if seen <= 0 || onlyA < 0 || onlyB < 0 {
		e.CI = Interval{e.N, e.N}
		return e
	}
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) // #nosec G404 -- a bootstrap needs a seeded generator, not a secure one.
	ests := make([]float64, Bootstraps)
	for b := range ests {
		var a, bb, both int
		for range seen {
			switch k := rng.IntN(seen); {
			case k < onlyA:
				a++
			case k < onlyA+onlyB:
				bb++
			default:
				both++
			}
		}
		ests[b] = Chapman(a+both, bb+both, both)
	}
	slices.Sort(ests)
	e.CI = Interval{quantile(ests, 0.025), quantile(ests, 0.975)}
	return e
}

// quantile gives the q quantile of the sorted values, with linear interpolation.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	pos := q * float64(len(sorted)-1)
	i := int(pos)
	if i+1 >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[i] + (pos-float64(i))*(sorted[i+1]-sorted[i])
}
