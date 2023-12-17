package utils

import (
	"github.com/anyongjin/banexg/log"
	"go.uber.org/zap"
	"testing"
)

type SignCase struct {
	data   string
	secret string
	method string
	hash   string
	digest string
	sign   string
}

var fakePrivatePem = `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----
`

func TestSignature(t *testing.T) {

	fakeSecret := "ThisIsASecret"

	items := []SignCase{
		{
			data:   "timestamp=1702376740732&recvWindow=10000",
			secret: fakeSecret,
			method: "hmac",
			hash:   "sha256",
			digest: "hex",
			sign:   "721c211bd113874ac03604e7d9cc23e8ac28d557f509749fd68461f326f555b3",
		},
		{
			data:   "timestamp=1702383061579&recvWindow=10000",
			secret: fakePrivatePem,
			method: "rsa",
			hash:   "sha256",
			sign:   "s65Oyv%2BBwC9MWryDBCytJ51x%2BYhWE20EC2c7BKGW%2BolHx%2B886uHVQ3O%2F5tT6NKQDWZJBQ0jYcSERGDWcMgXTjFdNztmRuTzSMPBlP56q4R8iEfYPNnc4z8W3UZMXfdlA72AREylggHhbY8y40ailx%2FSWoe2DwZOzMwh88kcH7xrovouGZzq1ocUSRX%2FTVdEpu%2BIoYuyL2Ug4zgZ9puVAbVCCC9oo9bUNdb00z4gHZy58DLpf0GWvc1vSEOHdHeqRQzTuP0KpC3PNxI%2B6g5E7FaTinO5OshXgPYExJKXjOfW%2BOrRvW%2F2FHos1EBP2btSGy5wluC5cvV8TAyBka5sNks8Ob1fD8Wu2ATRdMUchnw2M63fweFh4g0EnRGjbUHzt1WDGpbu8Uiqir%2BZpKY1hxh%2B6bqnPXVvRasnNMH9UwzTeI40pocJtjqfiRQLrZvuyJGL6IwsrLTTddqkmENL%2FSuViK21gq0YbjfMLHEUhtTGvdxWyTkA6ieRBYK8oWcGr",
		},
	}

	for _, item := range items {
		sign1, err := Signature(item.data, item.secret, item.method, item.hash, item.digest)
		if err == nil && sign1 == item.sign {
			log.Info("pass", zap.String("method", item.method), zap.String("sign", sign1))
		} else {
			log.Error("FAIL", zap.String("method", item.method), zap.String("sign", sign1),
				zap.String("expect", item.sign))
		}
	}

}
