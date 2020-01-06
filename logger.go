package sqlxx

import (
	"context"
	"io"
	"log"
)

type Logger interface {
	Debugf(ctx context.Context, format string, args ...interface{})
	Infof(ctx context.Context, format string, args ...interface{})
	Warnf(ctx context.Context, format string, args ...interface{})
	Errorf(ctx context.Context, format string, args ...interface{})
}

type LoggerImpl struct {
	debug, info, warn, err *log.Logger
}

func NewLogger(out io.Writer) Logger {
	return &LoggerImpl{
		debug: log.New(out, "[DEBUG] ", log.LstdFlags),
		info:  log.New(out, "[INFO] ", log.LstdFlags),
		warn:  log.New(out, "[WARN] ", log.LstdFlags),
		err:   log.New(out, "[ERROR] ", log.LstdFlags),
	}
}

func (li *LoggerImpl) Debugf(ctx context.Context, format string, args ...interface{}) {
	li.debug.Printf(format, args...)
}

func (li *LoggerImpl) Infof(ctx context.Context, format string, args ...interface{}) {
	li.info.Printf(format, args...)
}

func (li *LoggerImpl) Warnf(ctx context.Context, format string, args ...interface{}) {
	li.warn.Printf(format, args...)
}

func (li *LoggerImpl) Errorf(ctx context.Context, format string, args ...interface{}) {
	li.err.Printf(format, args...)
}
