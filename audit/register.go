package audit

import (
	"strings"
)

// record is one record of a register file.
type record struct {
	line   int
	fields []string
}

// readRegister returns the records of the register at rel. Blank lines and lines that start with # are not records. A register that does not exist has no records.
func readRegister(a *auditor, rel string) ([]record, error) {
	data, err := a.readFile(rel)
	if err != nil {
		return nil, err
	}
	var out []record
	n := 0
	for line := range strings.Lines(string(data)) {
		n++
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		out = append(out, record{line: n, fields: f})
	}
	return out, nil
}
