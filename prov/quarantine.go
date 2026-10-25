package prov

import (
	"maps"
	"slices"
)

// Hit is a field that an agent wrote, with the line of the agent.
type Hit struct {
	Tag  string
	Line Line
}

// Produced returns the fields in the logs that an agent wrote with the model and the prompt version, and that no person changed, confirmed or quarantined since (spec 06.5, withdrawal). An empty prompt matches each prompt version. A line that names no model or no prompt takes them from the summary of its run, when runs has it. The hits are in the order of tag and field.
func Produced(logs map[string][]Line, runs map[string]*Run, model, prompt string) []Hit {
	var out []Hit
	for _, t := range slices.Sorted(maps.Keys(logs)) {
		q := Quarantined(logs[t])
		u := Unconfirmed(logs[t])
		for _, f := range slices.Sorted(maps.Keys(u)) {
			l := u[f]
			if _, ok := q[f]; ok || !made(l, runs[l.Run], model, prompt) {
				continue
			}
			out = append(out, Hit{t, l})
		}
	}
	return out
}

// made reports whether the line of an agent is from the model and the prompt version.
func made(l Line, r *Run, model, prompt string) bool {
	m, p := l.Model == model, prompt == "" || l.Prompt == prompt
	if r != nil {
		m = m || l.Model == "" && slices.Contains(r.Models, model)
		p = p || l.Prompt == "" && r.Prompt == prompt
	}
	return m && p
}
