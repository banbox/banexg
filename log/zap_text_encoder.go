package log

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
	"math"
	"sync"
	"time"
	"unicode/utf8"
)

// DefaultTimeEncoder serializes time.Time to a human-readable formatted string
func DefaultTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	//s := t.Format("2006/01/02 15:04:05.000 -07:00")
	s := t.Format("2006/01/02 15:04:05.000")
	if e, ok := enc.(*TextEncoder); ok {
		for _, c := range []byte(s) {
			e.Buf.AppendByte(c)
		}
		return
	}
	enc.AppendString(s)
}

// ShortCallerEncoder serializes a caller in file:line format.
func ShortCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(caller.TrimmedPath())
}

// For JSON-escaping; see TextEncoder.safeAddString below.
const _hex = "0123456789abcdef"

var _textPool = sync.Pool{New: func() interface{} {
	return &TextEncoder{}
}}

var (
	_pool = buffer.NewPool()
	// Get retrieves a buffer from the pool, creating one if necessary.
	Get = _pool.Get
)

func getTextEncoder() *TextEncoder {
	return _textPool.Get().(*TextEncoder)
}

func putTextEncoder(enc *TextEncoder) {
	if enc.reflectBuf != nil {
		enc.reflectBuf.Free()
	}
	enc.EncoderConfig = nil
	enc.Buf = nil
	enc.spaced = false
	enc.openNamespaces = 0
	enc.reflectBuf = nil
	enc.reflectEnc = nil
	_textPool.Put(enc)
}

type TextEncoder struct {
	*zapcore.EncoderConfig
	Buf                 *buffer.Buffer
	spaced              bool // include spaces after colons and commas
	openNamespaces      int
	disableErrorVerbose bool

	// for encoding generic values by reflection
	reflectBuf *buffer.Buffer
	reflectEnc *json.Encoder
}

func NewTextEncoder(encoderConfig *zapcore.EncoderConfig, spaced bool, disableErrorVerbose bool) zapcore.Encoder {
	return &TextEncoder{
		EncoderConfig:       encoderConfig,
		Buf:                 _pool.Get(),
		spaced:              spaced,
		disableErrorVerbose: disableErrorVerbose,
	}
}

func NewEncoderConfig() *zapcore.EncoderConfig {
	return &zapcore.EncoderConfig{
		// Keys can be anything except the empty string.
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "name",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stack",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     DefaultTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   ShortCallerEncoder,
	}
}

// NewTextEncoderByConfig creates a fast, low-allocation Text encoder with config. The encoder
// appropriately escapes all field keys and values.
func NewTextEncoderByConfig(cfg *Config) zapcore.Encoder {
	cc := NewEncoderConfig()
	if cfg.DisableTimestamp {
		cc.TimeKey = ""
	}
	switch cfg.Format {
	case "text", "":
		return &TextEncoder{
			EncoderConfig:       cc,
			Buf:                 _pool.Get(),
			spaced:              false,
			disableErrorVerbose: cfg.DisableErrorVerbose,
		}
	case "json":
		return zapcore.NewJSONEncoder(*cc)
	default:
		panic(fmt.Sprintf("unsupport log format: %s", cfg.Format))
	}
}

func (enc *TextEncoder) Reset() {
	putTextEncoder(enc)
}

func (enc *TextEncoder) AddArray(key string, arr zapcore.ArrayMarshaler) error {
	enc.addKey(key)
	return enc.AppendArray(arr)
}

func (enc *TextEncoder) AddObject(key string, obj zapcore.ObjectMarshaler) error {
	enc.addKey(key)
	return enc.AppendObject(obj)
}

func (enc *TextEncoder) AddBinary(key string, val []byte) {
	enc.AddString(key, base64.StdEncoding.EncodeToString(val))
}

func (enc *TextEncoder) AddByteString(key string, val []byte) {
	enc.addKey(key)
	enc.AppendByteString(val)
}

func (enc *TextEncoder) AddBool(key string, val bool) {
	enc.addKey(key)
	enc.AppendBool(val)
}

func (enc *TextEncoder) AddComplex128(key string, val complex128) {
	enc.addKey(key)
	enc.AppendComplex128(val)
}

func (enc *TextEncoder) AddDuration(key string, val time.Duration) {
	enc.addKey(key)
	enc.AppendDuration(val)
}

func (enc *TextEncoder) AddFloat64(key string, val float64) {
	enc.addKey(key)
	enc.AppendFloat64(val)
}

func (enc *TextEncoder) AddInt64(key string, val int64) {
	enc.addKey(key)
	enc.AppendInt64(val)
}

func (enc *TextEncoder) resetReflectBuf() {
	if enc.reflectBuf == nil {
		enc.reflectBuf = _pool.Get()
		enc.reflectEnc = json.NewEncoder(enc.reflectBuf)
	} else {
		enc.reflectBuf.Reset()
	}
}

func (enc *TextEncoder) AddReflected(key string, obj interface{}) error {
	enc.resetReflectBuf()
	err := enc.reflectEnc.Encode(obj)
	if err != nil {
		return err
	}
	enc.reflectBuf.TrimNewline()
	enc.addKey(key)
	enc.AppendByteString(enc.reflectBuf.Bytes())
	return nil
}

func (enc *TextEncoder) OpenNamespace(key string) {
	enc.addKey(key)
	enc.Buf.AppendByte('{')
	enc.openNamespaces++
}

func (enc *TextEncoder) AddString(key, val string) {
	enc.addKey(key)
	enc.AppendString(val)
}

func (enc *TextEncoder) AddTime(key string, val time.Time) {
	enc.addKey(key)
	enc.AppendTime(val)
}

func (enc *TextEncoder) AddUint64(key string, val uint64) {
	enc.addKey(key)
	enc.AppendUint64(val)
}

func (enc *TextEncoder) AppendArray(arr zapcore.ArrayMarshaler) error {
	enc.addElementSeparator()
	ne := enc.cloned()
	ne.Buf.AppendByte('[')
	err := arr.MarshalLogArray(ne)
	ne.Buf.AppendByte(']')
	enc.AppendByteString(ne.Buf.Bytes())
	ne.Buf.Free()
	putTextEncoder(ne)
	return err
}

func (enc *TextEncoder) AppendObject(obj zapcore.ObjectMarshaler) error {
	enc.addElementSeparator()
	ne := enc.cloned()
	ne.Buf.AppendByte('{')
	err := obj.MarshalLogObject(ne)
	ne.Buf.AppendByte('}')
	enc.AppendByteString(ne.Buf.Bytes())
	ne.Buf.Free()
	putTextEncoder(ne)
	return err
}

func (enc *TextEncoder) AppendBool(val bool) {
	enc.addElementSeparator()
	enc.Buf.AppendBool(val)
}

func (enc *TextEncoder) AppendByteString(val []byte) {
	enc.addElementSeparator()
	if !enc.needDoubleQuotes(string(val)) {
		enc.safeAddByteString(val)
		return
	}
	enc.Buf.AppendByte('"')
	enc.safeAddByteString(val)
	enc.Buf.AppendByte('"')
}

func (enc *TextEncoder) AppendComplex128(val complex128) {
	enc.addElementSeparator()
	// Cast to a platform-independent, fixed-size type.
	r, i := real(val), imag(val)
	enc.Buf.AppendFloat(r, 64)
	enc.Buf.AppendByte('+')
	enc.Buf.AppendFloat(i, 64)
	enc.Buf.AppendByte('i')
}

func (enc *TextEncoder) AppendDuration(val time.Duration) {
	cur := enc.Buf.Len()
	enc.EncodeDuration(val, enc)
	if cur == enc.Buf.Len() {
		// User-supplied EncodeDuration is a no-op. Fall back to nanoseconds to keep
		// JSON valid.
		enc.AppendInt64(int64(val))
	}
}

func (enc *TextEncoder) AppendInt64(val int64) {
	enc.addElementSeparator()
	enc.Buf.AppendInt(val)
}

func (enc *TextEncoder) AppendReflected(val interface{}) error {
	enc.resetReflectBuf()
	err := enc.reflectEnc.Encode(val)
	if err != nil {
		return err
	}
	enc.reflectBuf.TrimNewline()
	enc.AppendByteString(enc.reflectBuf.Bytes())
	return nil
}

func (enc *TextEncoder) AppendString(val string) {
	enc.addElementSeparator()
	enc.safeAddStringWithQuote(val)
}

func (enc *TextEncoder) AppendTime(val time.Time) {
	cur := enc.Buf.Len()
	enc.EncodeTime(val, enc)
	if cur == enc.Buf.Len() {
		// User-supplied EncodeTime is a no-op. Fall back to nanos since epoch to keep
		// output JSON valid.
		enc.AppendInt64(val.UnixNano())
	}
}

func (enc *TextEncoder) beginQuoteFiled() {
	if enc.Buf.Len() > 0 {
		enc.Buf.AppendByte(' ')
	}
	//enc.Buf.AppendByte('[')
}

func (enc *TextEncoder) endQuoteFiled() {
	//enc.Buf.AppendByte(']')
}

func (enc *TextEncoder) AppendUint64(val uint64) {
	enc.addElementSeparator()
	enc.Buf.AppendUint(val)
}

func (enc *TextEncoder) AddComplex64(k string, v complex64) { enc.AddComplex128(k, complex128(v)) }

func (enc *TextEncoder) AddFloat32(k string, v float32) { enc.AddFloat64(k, float64(v)) }

func (enc *TextEncoder) AddInt(k string, v int) { enc.AddInt64(k, int64(v)) }

func (enc *TextEncoder) AddInt32(k string, v int32) { enc.AddInt64(k, int64(v)) }

func (enc *TextEncoder) AddInt16(k string, v int16) { enc.AddInt64(k, int64(v)) }

func (enc *TextEncoder) AddInt8(k string, v int8) { enc.AddInt64(k, int64(v)) }

func (enc *TextEncoder) AddUint(k string, v uint) { enc.AddUint64(k, uint64(v)) }

func (enc *TextEncoder) AddUint32(k string, v uint32) { enc.AddUint64(k, uint64(v)) }

func (enc *TextEncoder) AddUint16(k string, v uint16) { enc.AddUint64(k, uint64(v)) }

func (enc *TextEncoder) AddUint8(k string, v uint8) { enc.AddUint64(k, uint64(v)) }

func (enc *TextEncoder) AddUintptr(k string, v uintptr) { enc.AddUint64(k, uint64(v)) }

func (enc *TextEncoder) AppendComplex64(v complex64) { enc.AppendComplex128(complex128(v)) }

func (enc *TextEncoder) AppendFloat64(v float64) { enc.appendFloat(v, 64) }

func (enc *TextEncoder) AppendFloat32(v float32) { enc.appendFloat(float64(v), 32) }

func (enc *TextEncoder) AppendInt(v int) { enc.AppendInt64(int64(v)) }

func (enc *TextEncoder) AppendInt32(v int32) { enc.AppendInt64(int64(v)) }

func (enc *TextEncoder) AppendInt16(v int16) { enc.AppendInt64(int64(v)) }

func (enc *TextEncoder) AppendInt8(v int8) { enc.AppendInt64(int64(v)) }

func (enc *TextEncoder) AppendUint(v uint) { enc.AppendUint64(uint64(v)) }

func (enc *TextEncoder) AppendUint32(v uint32) { enc.AppendUint64(uint64(v)) }

func (enc *TextEncoder) AppendUint16(v uint16) { enc.AppendUint64(uint64(v)) }

func (enc *TextEncoder) AppendUint8(v uint8) { enc.AppendUint64(uint64(v)) }

func (enc *TextEncoder) AppendUintptr(v uintptr) { enc.AppendUint64(uint64(v)) }

func (enc *TextEncoder) Clone() zapcore.Encoder {
	clone := enc.cloned()
	clone.Buf.Write(enc.Buf.Bytes())
	return clone
}

func (enc *TextEncoder) cloned() *TextEncoder {
	clone := getTextEncoder()
	clone.EncoderConfig = enc.EncoderConfig
	clone.spaced = enc.spaced
	clone.openNamespaces = enc.openNamespaces
	clone.disableErrorVerbose = enc.disableErrorVerbose
	clone.Buf = _pool.Get()
	return clone
}

func (enc *TextEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	final := enc.cloned()
	if final.TimeKey != "" {
		final.beginQuoteFiled()
		final.AppendTime(ent.Time)
		final.endQuoteFiled()
	}

	if final.LevelKey != "" {
		final.beginQuoteFiled()
		cur := final.Buf.Len()
		final.EncodeLevel(ent.Level, final)
		if cur == final.Buf.Len() {
			// User-supplied EncodeLevel was a no-op. Fall back to strings to keep
			// output JSON valid.
			final.AppendString(ent.Level.String())
		}
		final.endQuoteFiled()
	}

	if ent.LoggerName != "" && final.NameKey != "" {
		final.beginQuoteFiled()
		cur := final.Buf.Len()
		nameEncoder := final.EncodeName

		// if no name encoder provided, fall back to FullNameEncoder for backwards
		// compatibility
		if nameEncoder == nil {
			nameEncoder = zapcore.FullNameEncoder
		}

		nameEncoder(ent.LoggerName, final)
		if cur == final.Buf.Len() {
			// User-supplied EncodeName was a no-op. Fall back to strings to
			// keep output JSON valid.
			final.AppendString(ent.LoggerName)
		}
		final.endQuoteFiled()
	}
	if ent.Caller.Defined && final.CallerKey != "" {
		final.beginQuoteFiled()
		cur := final.Buf.Len()
		final.EncodeCaller(ent.Caller, final)
		if cur == final.Buf.Len() {
			// User-supplied EncodeCaller was a no-op. Fall back to strings to
			// keep output JSON valid.
			final.AppendString(ent.Caller.String())
		}
		final.endQuoteFiled()
	}
	// add Message
	if len(ent.Message) > 0 {
		final.beginQuoteFiled()
		final.AppendString(ent.Message)
		final.endQuoteFiled()
	}
	if enc.Buf.Len() > 0 {
		final.Buf.AppendByte(' ')
		final.Buf.Write(enc.Buf.Bytes())
	}
	final.addFields(fields)
	final.closeOpenNamespaces()
	if ent.Stack != "" && final.StacktraceKey != "" {
		final.beginQuoteFiled()
		final.AddString(final.StacktraceKey, ent.Stack)
		final.endQuoteFiled()
	}

	if final.LineEnding != "" {
		final.Buf.AppendString(final.LineEnding)
	} else {
		final.Buf.AppendString(zapcore.DefaultLineEnding)
	}

	ret := final.Buf
	putTextEncoder(final)
	return ret, nil
}

func (enc *TextEncoder) truncate() {
	enc.Buf.Reset()
}

func (enc *TextEncoder) closeOpenNamespaces() {
	for i := 0; i < enc.openNamespaces; i++ {
		enc.Buf.AppendByte('}')
	}
}

func (enc *TextEncoder) addKey(key string) {
	enc.addElementSeparator()
	enc.safeAddStringWithQuote(key)
	enc.Buf.AppendByte('=')
}

func (enc *TextEncoder) addElementSeparator() {
	last := enc.Buf.Len() - 1
	if last < 0 {
		return
	}
	switch enc.Buf.Bytes()[last] {
	case '{', '[', ':', ',', ' ', '=':
		return
	default:
		enc.Buf.AppendByte(',')
	}
}

func (enc *TextEncoder) appendFloat(val float64, bitSize int) {
	enc.addElementSeparator()
	switch {
	case math.IsNaN(val):
		enc.Buf.AppendString("NaN")
	case math.IsInf(val, 1):
		enc.Buf.AppendString("+Inf")
	case math.IsInf(val, -1):
		enc.Buf.AppendString("-Inf")
	default:
		enc.Buf.AppendFloat(val, bitSize)
	}
}

// safeAddString JSON-escapes a string and appends it to the internal buffer.
// Unlike the standard library's encoder, it doesn't attempt to protect the
// user from browser vulnerabilities or JSONP-related problems.
func (enc *TextEncoder) safeAddString(s string) {
	for i := 0; i < len(s); {
		if enc.tryAddRuneSelf(s[i]) {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if enc.tryAddRuneError(r, size) {
			i++
			continue
		}
		enc.Buf.AppendString(s[i : i+size])
		i += size
	}
}

// safeAddStringWithQuote will automatically add quotoes.
func (enc *TextEncoder) safeAddStringWithQuote(s string) {
	if !enc.needDoubleQuotes(s) {
		enc.safeAddString(s)
		return
	}
	enc.Buf.AppendByte('"')
	enc.safeAddString(s)
	enc.Buf.AppendByte('"')
}

// safeAddByteString is no-alloc equivalent of safeAddString(string(s)) for s []byte.
func (enc *TextEncoder) safeAddByteString(s []byte) {
	for i := 0; i < len(s); {
		if enc.tryAddRuneSelf(s[i]) {
			i++
			continue
		}
		r, size := utf8.DecodeRune(s[i:])
		if enc.tryAddRuneError(r, size) {
			i++
			continue
		}
		enc.Buf.Write(s[i : i+size])
		i += size
	}
}

// See [log-fileds](https://github.com/tikv/rfcs/blob/master/text/2018-12-19-unified-log-format.md#log-fields-section).
func (enc *TextEncoder) needDoubleQuotes(s string) bool {
	// anyongjin: 这里固定不添加双引号
	//for i := 0; i < len(s); {
	//	b := s[i]
	//	if b <= 0x20 {
	//		return true
	//	}
	//	switch b {
	//	case '\\', '"', '[', ']', '=':
	//		return true
	//	}
	//	i++
	//}
	return false
}

// tryAddRuneSelf appends b if it is valid UTF-8 character represented in a single byte.
func (enc *TextEncoder) tryAddRuneSelf(b byte) bool {
	if b >= utf8.RuneSelf {
		return false
	}
	if 0x20 <= b && b != '\\' && b != '"' {
		enc.Buf.AppendByte(b)
		return true
	}
	switch b {
	case '\\', '"':
		enc.Buf.AppendByte('\\')
		enc.Buf.AppendByte(b)
	case '\n':
		enc.Buf.AppendByte('\n')
		//enc.Buf.AppendByte('\\')
		//enc.Buf.AppendByte('n')
	case '\r':
		enc.Buf.AppendByte('\r')
		//enc.Buf.AppendByte('\\')
		//enc.Buf.AppendByte('r')
	case '\t':
		enc.Buf.AppendByte('\t')
		//enc.Buf.AppendByte('\\')
		//enc.Buf.AppendByte('t')

	default:
		// Encode bytes < 0x20, except for the escape sequences above.
		enc.Buf.AppendString(`\u00`)
		enc.Buf.AppendByte(_hex[b>>4])
		enc.Buf.AppendByte(_hex[b&0xF])
	}
	return true
}

func (enc *TextEncoder) tryAddRuneError(r rune, size int) bool {
	if r == utf8.RuneError && size == 1 {
		enc.Buf.AppendString(`\ufffd`)
		return true
	}
	return false
}

func (enc *TextEncoder) addFields(fields []zapcore.Field) {
	for _, f := range fields {
		if f.Type == zapcore.ErrorType {
			// handle ErrorType in pingcap/log to fix "[key=?,keyVerbose=?]" problem.
			// see more detail at https://github.com/pingcap/log/pull/5
			enc.encodeError(f)
			continue
		}
		enc.beginQuoteFiled()
		f.AddTo(enc)
		enc.endQuoteFiled()
	}
}

func (enc *TextEncoder) encodeError(f zapcore.Field) {
	err := f.Interface.(error)
	basic := err.Error()
	enc.beginQuoteFiled()
	enc.AddString(f.Key, basic)
	enc.endQuoteFiled()
	if enc.disableErrorVerbose {
		return
	}
	if e, isFormatter := err.(fmt.Formatter); isFormatter {
		verbose := fmt.Sprintf("%+v", e)
		if verbose != basic {
			// This is a rich error type, like those produced by
			// errors.
			enc.beginQuoteFiled()
			enc.AddString(f.Key+"Verbose", verbose)
			enc.endQuoteFiled()
		}
	}
}
