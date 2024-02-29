package errs

import (
	"fmt"
	"runtime"
	"strings"
)

func NewMsg(code int, format string, a ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, a...), Stack: CallStack(3, 30)}
}

func New(code int, err error) *Error {
	return &Error{Code: code, Msg: err.Error(), Stack: CallStack(3, 30)}
}

func (e *Error) Short() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Msg)
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("[%d] %s\n%s", e.Code, e.Msg, e.Stack)
}

func CallStack(skip, maxNum int) string {
	pc := make([]uintptr, maxNum)
	n := runtime.Callers(skip, pc)
	frames := runtime.CallersFrames(pc[:n])
	var texts = make([]string, 0, 16)
	for {
		f, more := frames.Next()
		texts = append(texts, fmt.Sprintf("  at %v:%v %v", f.File, f.Line, f.Function))
		if !more {
			break
		}
	}
	return strings.Join(texts, "\n")
}
