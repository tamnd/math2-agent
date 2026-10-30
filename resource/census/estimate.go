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

// Sample is the evidence of one stratum for one pair of catalogues.
type Sample struct {
	NA, NB  float64 // the weighted in works of the stratum in A and in B
	KA, KB  int     // the records of the sample of A and of B behind NA and NB, or 0 for a catalogue that holds all its records
	Sampled int     // the records of the sample of A in the stratum that were matched against B
	Matched int     // the records of these that B has
	WB      float64 // the weight that corrects a match in the records of B, 1 for a lookup
}

// m gives the works in both catalogues.
func (s Sample) m(na, nb float64, matched int) float64 {
	return min(na, nb, na*fraction(matched, s.Sampled, s.WB))
}

// PairSample gives the Chapman estimate of a stratum from samples, and its 95% interval by a bootstrap over the samples (RS11.4.1). Each round draws the records of the sample of A, of the sample of B and of the matched sample again, so the interval holds the error of each sample, and a catalogue that holds all its records adds no error. The same seed gives the same interval.
func PairSample(s Sample, seed uint64) Estimate {
	nA, nB, m := round(s.NA), round(s.NB), round(s.m(s.NA, s.NB, s.Matched))
	e := Estimate{NA: nA, NB: nB, M: m, N: Chapman(nA, nB, m)}
	if s.Sampled <= 0 {
		e.CI = Interval{e.N, e.N}
		return e
	}
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) // #nosec G404 -- a bootstrap needs a seeded generator, not a secure one.
	p := float64(s.Matched) / float64(s.Sampled)
	ests := make([]float64, Bootstraps)
	for b := range ests {
		na, nb := redraw(rng, s.NA, s.KA), redraw(rng, s.NB, s.KB)
		matched := binomial(rng, s.Sampled, p)
		ests[b] = Chapman(round(na), round(nb), round(s.m(na, nb, matched)))
	}
	slices.Sort(ests)
	e.CI = Interval{quantile(ests, 0.025), quantile(ests, 0.975)}
	return e
}

// redraw gives the weighted count n of k records of a sample with a new count of records, drawn from the Poisson law with the mean k. A count with no sample (k is 0) stays.
func redraw(rng *rand.Rand, n float64, k int) float64 {
	if k <= 0 {
		return n
	}
	return n * float64(poisson(rng, float64(k))) / float64(k)
}

// poisson draws from the Poisson law with the mean mu: by the method of Knuth for a small mean, and by the normal law for a large mean.
func poisson(rng *rand.Rand, mu float64) int {
	if mu > 30 {
		return max(0, int(math.Round(mu+math.Sqrt(mu)*rng.NormFloat64())))
	}
	limit, k, prod := math.Exp(-mu), 0, rng.Float64()
	for prod > limit {
		k++
		prod *= rng.Float64()
	}
	return k
}

// binomial draws the successes of n trials with the chance p: one by one for a small n, and by the normal law for a large n.
func binomial(rng *rand.Rand, n int, p float64) int {
	if n <= 50 {
		k := 0
		for range n {
			if rng.Float64() < p {
				k++
			}
		}
		return k
	}
	mu := float64(n) * p
	return min(n, max(0, int(math.Round(mu+math.Sqrt(mu*(1-p))*rng.NormFloat64()))))
}
