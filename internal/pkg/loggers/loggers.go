package loggers

import "github.com/Educentr/go-project-starter/internal/pkg/ds"

var LoggerMapping = map[string]ds.Logger{
	"zerolog": &ZlogLogger{},
	"logrus":  &LogrusLogger{},
}
