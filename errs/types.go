package errs

type Error struct {
	Code  int
	msg   string
	Stack string
	err   error
	// BizCode is deprecated. Exchange-native codes are no longer exposed here;
	// callers should branch on the exchange-neutral Code field.
	BizCode int
	Data    interface{}
}
