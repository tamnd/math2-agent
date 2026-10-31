// Package understand has the output schemas of the passes of the role understand (corpus arxiv AX7.1), one file PASS.v1.json for each pass. The tool of a pass gives the schema of its pass to the model as the schema of its input, and the code of the pass checks each result again.
package understand

import (
	"embed"
	"encoding/json"
	"slices"
)

//go:embed *.v1.json
var files embed.FS

// File returns the name of the schema file of the pass.
func File(pass string) string { return pass + ".v1.json" }

// Of returns the schema of the pass, or nil when the pass has none.
func Of(pass string) json.RawMessage {
	data, err := files.ReadFile(File(pass))
	if err != nil {
		return nil
	}
	return slices.Clone(data)
}
