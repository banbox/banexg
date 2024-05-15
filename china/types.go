package china

import "github.com/banbox/banexg"

type China struct {
	*banexg.Exchange
}

type Exchange struct {
	Code       string `yaml:"code"`
	Title      string `yaml:"title"`
	IndexUrl   string `yaml:"index"`
	Suffix     string `yaml:"suffix"`
	CaseLower  bool   `yaml:"case_lower"`  // 品种ID是否小写
	DateNum    int    `yaml:"date_num"`    // 年月显示后几位？4或3
	OptionDash bool   `yaml:"option_dash"` // 期权C/P左右两侧是否有短横线
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
	Multiplier  float64  `yaml:"multiplier"`    // 合约乘数；价格单位是吨，每手含multiplier吨
	PriceTick   float64  `yaml:"price_tick"`    // 最小价格变动，单位：吨
	LimitChgPct float64  `yaml:"limit_chg_pct"` // 涨跌停板，单位：百分比
	MarginPct   float64  `yaml:"margin_pct"`    // 保证金比率，单位：百分比
}

type Fee struct {
	Unit  string  `yaml:"unit"`
	Val   float64 `yaml:"val"`
	ValCT float64 `yaml:"val_ct"` // 平今
	ValTD float64 `yaml:"val_td"` // 日内
}

type CnMarkets struct {
	Exchanges map[string]*Exchange `yaml:"exchanges"`
	Contracts []*ItemMarket        `yaml:"contracts"`
	Stocks    []*ItemMarket        `yaml:"stocks"`
}
