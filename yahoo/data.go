package yahoo

const (
	HostQuery1 = "query1"
	HostQuery2 = "query2"

	MidChartGet = "ChartGet"
	MidQuoteGet = "QuoteGet"
)

// Yahoo blocks the default `Go-http-client/1.1` UA with HTTP 403,
// so we send a real browser-style UA by default.
const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
