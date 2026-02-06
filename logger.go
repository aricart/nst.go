package nst

import (
	"fmt"
	"sync"

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
func (l *NilLogger) Errorf(format string, v ...interface{}) {}
func (l *NilLogger) Debugf(format string, v ...interface{}) {}
func (l *NilLogger) Tracef(format string, v ...interface{}) {}

// CaptureLogger implements server.Logger and captures all messages.
type CaptureLogger struct {
	mu       sync.Mutex
	messages []string
}

func NewCaptureLogger() *CaptureLogger {
	return &CaptureLogger{}
}

func (l *CaptureLogger) add(format string, v ...interface{}) {
	l.mu.Lock()
	l.messages = append(l.messages, fmt.Sprintf(format, v...))
	l.mu.Unlock()
}

// Messages returns a copy of all captured messages.
func (l *CaptureLogger) Messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	m := make([]string, len(l.messages))
	copy(m, l.messages)
	return m
}

func (l *CaptureLogger) Noticef(format string, v ...interface{}) { l.add(format, v...) }
func (l *CaptureLogger) Warnf(format string, v ...interface{})   { l.add(format, v...) }
func (l *CaptureLogger) Fatalf(format string, v ...interface{})  { l.add(format, v...) }
func (l *CaptureLogger) Errorf(format string, v ...interface{})  { l.add(format, v...) }
func (l *CaptureLogger) Debugf(format string, v ...interface{})  { l.add(format, v...) }
func (l *CaptureLogger) Tracef(format string, v ...interface{})  { l.add(format, v...) }
