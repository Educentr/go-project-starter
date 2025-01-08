package logger

import (
	"context"
	"errors"
)

type ctxKey int

const (
	loggerContextKey ctxKey = iota
)

var ErrLoggerNotFoundInContext = errors.New("logger not found in context")

func CopyLoggerContext(source context.Context, destination context.Context) (context.Context, error) {
	ILogg := source.Value(loggerContextKey)
	if ILogg == nil {
		return destination, ErrLoggerNotFoundInContext
	}

	return context.WithValue(destination, loggerContextKey, ILogg), nil
}

func Wrap(ctx context.Context, logger any) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}
