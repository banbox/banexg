package china

import "github.com/banbox/banexg"

type China struct {
	*banexg.Exchange
}

type ItemMarket struct {
	Code        string   `yaml:"code"`
	Title       string   `yaml:"title"`
	Market      string   `yaml:"market"`
	Exchange    string   `yaml:"exchange"`
	Extend      string   `yaml:"extend"`
	Alias       []string `yaml:"alias"`
	DayRanges   []string `yaml:"day_ranges"`
	NightRanges []string `yaml:"night_ranges"`
	Fee         *Fee     `yaml:"fee"`
	FeeCT       *Fee     `yaml:"fee_ct"`
	PriceTick   float64  `yaml:"price_tick"`
	LimitChgPct float64  `yaml:"limit_chg_pct"`
	MarginPct   float64  `yaml:"margin_pct"`
}

type Fee struct {
	Unit string  `yaml:"unit"`
	Val  float64 `yaml:"val"`
}

type CnMarkets struct {
	Goods []*ItemMarket `yaml:"goods"`
}
