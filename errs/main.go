package errs

import "fmt"

func NewMsg(code int, format string, a ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, a...)}
}

func New(code int, err error) *Error {
	return &Error{Code: code, Msg: err.Error()}
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Msg)
}
