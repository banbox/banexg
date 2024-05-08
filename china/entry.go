package china

import (
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
)

func New(Options map[string]interface{}) (*China, *errs.Error) {
	exg := &China{
		Exchange: &banexg.Exchange{
			ID:        "china",
			Name:      "China",
			Countries: []string{"CN"},
			RateLimit: 50,
			Options:   Options,
			Hosts:     &banexg.ExgHosts{},
			Fees: &banexg.ExgFee{
				Linear: &banexg.TradeFee{
					FeeSide:    "quote",
					TierBased:  false,
					Percentage: true,
					Taker:      0.0002,
					Maker:      0.0002,
				},
			},
			Apis: map[string]banexg.Entry{
				"test": {Path: "", Host: "", Method: "GET"},
			},
		},
	}
	err := exg.Init()
	return exg, err
}

func NewExchange(Options map[string]interface{}) (banexg.BanExchange, *errs.Error) {
	return New(Options)
}
