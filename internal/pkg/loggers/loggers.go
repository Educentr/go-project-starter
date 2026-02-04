package loggers

import "github.com/Educentr/go-project-starter/internal/pkg/ds"

// Shared constants for logger implementations
const (
	paramParts = 3
	errParam   = "err"
	nilErr     = "nil"
	destDir    = "pkg/app/logger"
)

var LoggerMapping = map[string]ds.Logger{
	"zerolog": &ZlogLogger{},
	"logrus":  &LogrusLogger{},
}
