package evalrt

import (
	"embed"
	"io/fs"
)

// The rt_*.go sources are embedded so the generated runner package is fully
// self-contained: codegen writes them next to the generated main.go with the
// package clause rewritten, giving the runner zero dependency on the replai
// module itself.
//
//go:embed rt_*.go
var rtFS embed.FS

// Sources returns the embedded runtime sources keyed by filename.
func Sources() (map[string][]byte, error) {
	entries, err := fs.ReadDir(rtFS, ".")
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		data, err := rtFS.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		out[e.Name()] = data
	}
	return out, nil
}
