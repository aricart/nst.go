package nst

import (
	nslogger "github.com/nats-io/nats-server/v2/logger"
	"github.com/nats-io/nats-server/v2/server"
)

type NilLogger struct {
	logger server.Logger
}

func NewNilLogger() server.Logger {
	logger := nslogger.NewStdLogger(true, true, true, true, true)
	return &NilLogger{
		logger: logger,
	}
}

func (l *NilLogger) Noticef(format string, v ...interface{}) {}
func (l *NilLogger) Warnf(format string, v ...interface{})   {}
func (l *NilLogger) Fatalf(format string, v ...interface{}) {
	l.logger.Fatalf(format, v...)
}

func (l *NilLogger) Errorf(format string, v ...interface{}) {
	l.logger.Errorf(format, v...)
}
func (l *NilLogger) Debugf(format string, v ...interface{}) {}
func (l *NilLogger) Tracef(format string, v ...interface{}) {}
