package loggers

import "github.com/Educentr/go-project-starter/internal/pkg/ds"

// Shared constants for logger implementations
const (
	paramParts = 3
	errParam   = "err"
	nilErr     = "nil"
	destDir    = "pkg/app/logger"

	// Names of the logger implementations, as written in project.yaml.
	zerologName = "zerolog"
	logrusName  = "logrus"
)

// Get returns the logger implementation registered under name, and whether
// there is one.
//
// A function rather than an exported map: the entries are shared, stateless
// implementations, and an exported map hands every importer the ability to
// replace one — which would change the logger every later generation emits. A
// map is also the one shape that cannot be a const, so there is nothing to
// assert about it being read-only; not exporting it is what makes that true.
func Get(name string) (ds.Logger, bool) {
	switch name {
	case zerologName:
		return &ZlogLogger{}, true
	case logrusName:
		return &LogrusLogger{}, true
	}

	return nil, false
}

// Names returns the registered logger names, in a stable order.
func Names() []string {
	return []string{logrusName, zerologName}
}
