package status

import "fmt"

// Level is an evidence level of spec 04.7. A greater Level is stronger evidence.
type Level int8

// The evidence levels, from weak to strong. None is the level of a node that has no evidence level, for example a concept.
const (
	None      Level = iota
	Refuted         // E⊥: a counterexample or a proof of the negation at E2 or above exists
	Open            // E0: a conjecture or a problem that nobody has resolved
	Claimed         // E1: somebody claims it, and nobody has checked the claim
	Standard        // E2: two independent textbook sources with locators
	Refereed        // E3: a source in a refereed venue with a locator
	Certified       // E4: a checked certificate, or a kernel proof that uses native_decide
	Kernel          // E5: a kernel proof of a faithful formal statement, with replay
)

var levelNames = [...]string{"", "E⊥", "E0", "E1", "E2", "E3", "E4", "E5"}

var levelWords = [...]string{"", "refuted", "open", "claimed", "standard", "refereed", "certified", "kernel"}

// String returns the short name of the level, for example "E5", or "" for None.
func (l Level) String() string {
	if l < None || l > Kernel {
		return fmt.Sprintf("Level(%d)", int8(l))
	}
	return levelNames[l]
}

// Word returns the name of the level in spec 04.7, for example "kernel".
func (l Level) Word() string {
	if l < None || l > Kernel {
		return l.String()
	}
	return levelWords[l]
}

// MarshalText gives the short name of the level.
func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

// UnmarshalText reads the short name of a level.
func (l *Level) UnmarshalText(b []byte) error {
	for i, n := range levelNames {
		if n == string(b) {
			*l = Level(i)
			return nil
		}
	}
	return fmt.Errorf("status: %q is not an evidence level", b)
}

// Fidelity is a fidelity level of spec 04.7. A greater Fidelity is a stronger belief that the formal statement says what the informal statement says.
type Fidelity int8

// The fidelity levels. Unbound is the level of a node with no formal binding.
const (
	Unbound Fidelity = iota - 1
	F0               // bound, and nobody compared the statements
	F1               // the machine checks agree
	F2               // one person who is not the author reviewed this exact statement hash
	F3               // two independent reviews agree
)

// String returns the name of the level, for example "F2", or "" for Unbound.
func (f Fidelity) String() string {
	switch {
	case f == Unbound:
		return ""
	case f >= F0 && f <= F3:
		return fmt.Sprintf("F%d", int8(f))
	}
	return fmt.Sprintf("Fidelity(%d)", int8(f))
}

// MarshalText gives the name of the level.
func (f Fidelity) MarshalText() ([]byte, error) { return []byte(f.String()), nil }

// UnmarshalText reads the name of a level.
func (f *Fidelity) UnmarshalText(b []byte) error {
	for g := Unbound; g <= F3; g++ {
		if g.String() == string(b) {
			*f = g
			return nil
		}
	}
	return fmt.Errorf("status: %q is not a fidelity level", b)
}
