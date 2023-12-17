package utils

import "errors"

var (
	ErrUnSupportSign    = errors.New("unsupported sign method")
	ErrInvalidTimeFrame = errors.New("invalid timeframe")
	tfSecsCache         = make(map[string]int)
)

const (
	UriEncodeSafe = "~()*!.'"
)
