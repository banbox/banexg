package log

import (
	"go.uber.org/zap"
	"sync/atomic"
)

// MLogger is a wrapper type of zap.Logger.
type MLogger struct {
	*zap.Logger
	rl atomic.Value // *utils.ReconfigurableRateLimiter
}

// With encapsulates zap.Logger With method to return MLogger instance.
func (l *MLogger) With(fields ...zap.Field) *MLogger {
	nl := &MLogger{
		Logger: l.Logger.With(fields...),
	}
	return nl
}
