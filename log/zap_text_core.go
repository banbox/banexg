package log

import (
	"fmt"
	"go.uber.org/zap/zapcore"
)

// NewTextCore creates a Core that writes logs to a WriteSyncer.
func NewTextCore(enc zapcore.Encoder, ws zapcore.WriteSyncer, enab zapcore.LevelEnabler) zapcore.Core {
	return &textIOCore{
		LevelEnabler: enab,
		enc:          enc,
		out:          ws,
	}
}

// textIOCore is a copy of zapcore.ioCore that only accept *TextEncoder
// it can be removed after https://github.com/uber-go/zap/pull/685 be merged
type textIOCore struct {
	zapcore.LevelEnabler
	enc zapcore.Encoder
	out zapcore.WriteSyncer
}

func (c *textIOCore) With(fields []zapcore.Field) zapcore.Core {
	clone := c.clone()
	// it's different to ioCore, here call TextEncoder#addFields to fix https://github.com/pingcap/log/issues/3
	switch e := clone.enc.(type) {
	case *TextEncoder:
		e.addFields(fields)
	case zapcore.ObjectEncoder:
		for _, field := range fields {
			field.AddTo(e)
		}
	default:
		panic(fmt.Sprintf("unsupported encode type: %T for With operation", clone.enc))
	}
	return clone
}

func (c *textIOCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *textIOCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	buf, err := c.enc.EncodeEntry(ent, fields)
	if err != nil {
		return err
	}
	_, err = c.out.Write(buf.Bytes())
	buf.Free()
	if err != nil {
		return err
	}
	if ent.Level > zapcore.ErrorLevel {
		// Since we may be crashing the program, sync the output. Ignore Sync
		// errors, pending a clean solution to issue https://github.com/uber-go/zap/issues/370.
		c.Sync()
	}
	return nil
}

func (c *textIOCore) Sync() error {
	return c.out.Sync()
}

func (c *textIOCore) clone() *textIOCore {
	return &textIOCore{
		LevelEnabler: c.LevelEnabler,
		enc:          c.enc.Clone(),
		out:          c.out,
	}
}
