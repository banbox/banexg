package errs

type Error struct {
	Code  int    `json:"code"`
	Msg   string `json:"msg"`
	Stack string `json:"stack"`
	Data  interface{}
}
