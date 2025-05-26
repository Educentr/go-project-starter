package loggers

import "gitlab.educentr.info/golang/service-starter/internal/pkg/ds"

var LoggerMapping = map[string]ds.Logger{
	"zerolog": &ZlogLogger{},
}
