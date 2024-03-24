package errs

type Error struct {
	Code    int
	msg     string
	Stack   string
	err     error
	BizCode int
	Data    interface{}
}
