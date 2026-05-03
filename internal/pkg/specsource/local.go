package specsource

import (
	"path/filepath"
)

// LocalSource is a path on the local filesystem. RawPath is the value as it
// appeared in project.yaml; resolving joins it with the config base directory
// (preserving the pre-feature behavior exactly).
type LocalSource struct {
	RawPath string
}

func (s LocalSource) Kind() Kind { return KindLocal }

func (s LocalSource) TargetFilename() string {
	return filepath.Base(s.RawPath)
}

func (s LocalSource) Describe() string {
	return s.RawPath
}
