package utils

var (
	tfSecsCache = make(map[string]int)
)

const (
	UriEncodeSafe = "~()*!.'"
)
