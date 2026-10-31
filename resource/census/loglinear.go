package census

import (
	"cmp"
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
)

// Histories are the counts of the capture histories of k catalogues (RS11.4.2). The index is a bit mask: bit i is set when catalogue i holds the work. The count at index 0 is the works that no catalogue holds, which is not observed, so the fit does not read it.
type Histories []float64

// NewHistories gives the zero counts of k catalogues.
func NewHistories(k int) Histories { return make(Histories, 1<<k) }

// K gives the count of catalogues.
func (h Histories) K() int { return bits.Len(uint(len(h))) - 1 }

// Observed gives the count of the works that at least one catalogue holds.
func (h Histories) Observed() float64 {
	n := 0.0
	for _, c := range h[1:] {
		n += c
	}
	return n
}

// Model is one log-linear model of the capture histories. Each model has the intercept. The names follow Rcapture.
type Model struct {
	Name    string
	Main    bool     // a term for each catalogue (Mt). Without it, one term for the count of captures (M0)
	Darroch bool     // the term of Darroch for heterogeneity: the square of the count of captures over 2 (Mh, Mth)
	Pairs   [][2]int // the pairwise interaction terms
}

// String gives the name of the model and its pairs, for example Mt+[0,1].
func (m Model) String() string {
	s := m.Name
	for _, p := range m.Pairs {
		s += fmt.Sprintf("+[%d,%d]", p[0], p[1])
	}
	return s
}

// columns gives the row of the design matrix for the history mask.
func (m Model) columns(k int, mask int) []float64 {
	c := bits.OnesCount(uint(mask))
	row := []float64{1}
	if m.Main {
		for i := range k {
			row = append(row, float64(mask>>i&1))
		}
	} else {
		row = append(row, float64(c))
	}
	if m.Darroch {
		row = append(row, float64(c*c)/2)
	}
	for _, p := range m.Pairs {
		row = append(row, float64((mask>>p[0])&(mask>>p[1])&1))
	}
	return row
}

// Models gives the models that the fit compares for k catalogues: M0, Mt, Mh and Mth, and Mt with each one pairwise term. With four or more catalogues it adds Mt with all pairwise terms. Each pair of forced is in each model, so that two dependent catalogues always have their term (RS11.4.3). A model with more terms than the observed histories is left out, so the highest-order interaction is never in a model.
func Models(k int, forced [][2]int) []Model {
	var pairs [][2]int
	for i := range k {
		for j := i + 1; j < k; j++ {
			pairs = append(pairs, [2]int{i, j})
		}
	}
	base := []Model{{Name: "M0"}, {Name: "Mt", Main: true}, {Name: "Mh", Darroch: true}, {Name: "Mth", Main: true, Darroch: true}}
	var out []Model
	add := func(m Model) {
		for _, p := range forced {
			if !slices.Contains(m.Pairs, p) {
				m.Pairs = append(m.Pairs, p)
			}
		}
		if len(m.Pairs) > 0 && !m.Main {
			return // a pairwise term needs the main terms
		}
		slices.SortFunc(m.Pairs, func(x, y [2]int) int { return cmp.Or(cmp.Compare(x[0], y[0]), cmp.Compare(x[1], y[1])) })
		m.Pairs = slices.Compact(m.Pairs)
		if len(m.columns(k, 0)) > 1<<k-1 {
			return
		}
		for _, o := range out {
			if o.String() == m.String() {
				return
			}
		}
		out = append(out, m)
	}
	for _, m := range base {
		add(m)
	}
	for _, p := range pairs {
		add(Model{Name: "Mt", Main: true, Pairs: [][2]int{p}})
	}
	if k >= 4 {
		add(Model{Name: "Mt", Main: true, Pairs: pairs})
	}
	return out
}

// Fit is the fit of one model.
type Fit struct {
	Model Model
	N     float64 // the estimate of the universe: the observed works and the fitted count of history 0
	AIC   float64
}

// design gives the design matrix and the counts of the observed histories.
func design(h Histories, m Model) (x [][]float64, y []float64) {
	for mask := 1; mask < len(h); mask++ {
		x = append(x, m.columns(h.K(), mask))
		y = append(y, h[mask])
	}
	return x, y
}

// fit fits the model to the observed histories. It gives false when the fit does not converge or the estimate is not finite.
func fit(h Histories, m Model) (Fit, bool) {
	x, y := design(h, m)
	beta, ok := fitBeta(x, y)
	if !ok {
		return Fit{}, false
	}
	unseen := math.Exp(beta[0])
	if math.IsNaN(unseen) || math.IsInf(unseen, 0) || unseen > 1e12 {
		return Fit{}, false
	}
	ll := 0.0
	for i := range y {
		mu := math.Exp(dot(x[i], beta))
		lg, _ := math.Lgamma(y[i] + 1)
		ll += y[i]*math.Log(mu) - mu - lg
	}
	return Fit{Model: m, N: h.Observed() + unseen, AIC: -2*ll + 2*float64(len(beta))}, true
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// solve solves a x = b by Gaussian elimination with partial pivoting. It gives false when a is singular.
func solve(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	for c := range n {
		piv := c
		for r := c + 1; r < n; r++ {
			if math.Abs(a[r][c]) > math.Abs(a[piv][c]) {
				piv = r
			}
		}
		if math.Abs(a[piv][c]) < 1e-12 {
			return nil, false
		}
		a[c], a[piv] = a[piv], a[c]
		b[c], b[piv] = b[piv], b[c]
		for r := c + 1; r < n; r++ {
			f := a[r][c] / a[c][c]
			for j := c; j < n; j++ {
				a[r][j] -= f * a[c][j]
			}
			b[r] -= f * b[c]
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		s := b[r]
		for j := r + 1; j < n; j++ {
			s -= a[r][j] * x[j]
		}
		x[r] = s / a[r][r]
	}
	return x, true
}

// LogLinear is the estimate of the universe from three or more catalogues.
type LogLinear struct {
	Model    string   // the model that AIC chose
	Observed float64  // the works that at least one catalogue holds
	N        float64  // the estimate of the universe
	CI       Interval // the 95% interval, by a parametric bootstrap of the chosen model
	Fits     []Fit    // each model that converged, by AIC
}

// EstimateLogLinear fits each model of Models to the histories and chooses the model with the least AIC (RS11.4.2). The interval comes from Bootstraps rounds: each round draws each observed count from a Poisson law with the fitted mean, and fits the chosen model again. It gives false when there are fewer than three catalogues or no model converges.
func EstimateLogLinear(h Histories, forced [][2]int, seed uint64) (LogLinear, bool) {
	if h.K() < 3 || h.Observed() == 0 {
		return LogLinear{}, false
	}
	var fits []Fit
	for _, m := range Models(h.K(), forced) {
		if f, ok := fit(h, m); ok {
			fits = append(fits, f)
		}
	}
	if len(fits) == 0 {
		return LogLinear{}, false
	}
	sort.SliceStable(fits, func(i, j int) bool { return fits[i].AIC < fits[j].AIC })
	best := fits[0]
	out := LogLinear{Model: best.Model.String(), Observed: h.Observed(), N: best.N, Fits: fits}
	means := fitted(h, best.Model)
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) // #nosec G404 -- a bootstrap needs a seeded generator, not a secure one.
	ns := make([]float64, 0, Bootstraps)
	for range Bootstraps {
		draw := NewHistories(h.K())
		for mask := 1; mask < len(h); mask++ {
			draw[mask] = float64(poisson(rng, means[mask]))
		}
		if draw.Observed() == 0 {
			continue
		}
		if f, ok := fit(draw, best.Model); ok {
			ns = append(ns, f.N)
		}
	}
	if len(ns) > 0 {
		slices.Sort(ns)
		out.CI = Interval{Low: quantile(ns, 0.025), High: quantile(ns, 0.975)}
	} else {
		out.CI = Interval{Low: best.N, High: best.N}
	}
	return out, true
}

// fitted gives the fitted mean of each history under the model.
func fitted(h Histories, m Model) Histories {
	out := NewHistories(h.K())
	x, y := design(h, m)
	beta, ok := fitBeta(x, y)
	if !ok {
		copy(out, h)
		return out
	}
	for i := range y {
		out[i+1] = math.Exp(dot(x[i], beta))
	}
	return out
}

// fitBeta gives the coefficients of the Poisson fit of y on x, by iteratively reweighted least squares. It gives false when the fit does not converge.
func fitBeta(x [][]float64, y []float64) ([]float64, bool) {
	p := len(x[0])
	beta := make([]float64, p)
	mu := make([]float64, len(y))
	eta := make([]float64, len(y))
	for i := range y {
		mu[i] = y[i] + 0.5
		eta[i] = math.Log(mu[i])
	}
	for range 100 {
		xtwx := make([][]float64, p)
		xtwz := make([]float64, p)
		for a := range p {
			xtwx[a] = make([]float64, p)
		}
		for i := range y {
			z := eta[i] + (y[i]-mu[i])/mu[i]
			for a := range p {
				xtwz[a] += x[i][a] * mu[i] * z
				for b := range p {
					xtwx[a][b] += x[i][a] * mu[i] * x[i][b]
				}
			}
		}
		next, ok := solve(xtwx, xtwz)
		if !ok {
			return nil, false
		}
		delta := 0.0
		for a := range p {
			delta = max(delta, math.Abs(next[a]-beta[a]))
		}
		beta = next
		for i := range y {
			eta[i] = dot(x[i], beta)
			mu[i] = math.Exp(eta[i])
		}
		if delta < 1e-9 {
			return beta, true
		}
	}
	return nil, false
}

// Lookups are the sources whose records come from a lookup of the resources that other sources give, by their ids or by their titles. Such a source is not a capture (RS11.4): it never holds a work that no other source holds, so the estimate of the works that only it holds would be zero.
var Lookups = map[string]bool{"s2": true, "zbmath_search": true, "arxiv_text": true, "crossref_language": true}

// Captures merges the catalogues that share lineage into one capture (RS11.4.3 rule 1). Two catalogues share lineage when one names the other in its lineage, or when both name the same catalogue. It gives the name of the capture of each catalogue: the names of its catalogues in order, joined by "+".
func Captures(catalogues []string, lineage map[string][]string) map[string]string {
	parent := map[string]string{}
	var find func(string) string
	find = func(s string) string {
		if parent[s] == "" || parent[s] == s {
			parent[s] = s
			return s
		}
		parent[s] = find(parent[s])
		return parent[s]
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[max(ra, rb)] = min(ra, rb)
		}
	}
	for _, c := range catalogues {
		find(c)
		for _, l := range lineage[c] {
			union(c, l)
		}
	}
	groups := map[string][]string{}
	for _, c := range catalogues {
		groups[find(c)] = append(groups[find(c)], c)
	}
	out := map[string]string{}
	for _, g := range groups {
		slices.Sort(g)
		name := strings.Join(g, "+")
		for _, c := range g {
			out[c] = name
		}
	}
	return out
}
