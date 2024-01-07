package base

import (
	"go.uber.org/zap/zapcore"
	"net/http"
	"strings"
)

type HttpHeader http.Header

func (h HttpHeader) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for k, v := range h {
		enc.AddString(k, strings.Join(v, ","))
	}
	return nil
}
