package errs

type Error struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}
