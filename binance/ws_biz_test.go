package binance

import (
	"fmt"
	"github.com/anyongjin/banexg/log"
	"github.com/h2non/gock"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"testing"
)

func TestWatchOhlcvs(t *testing.T) {
	gock.DisableNetworking()
	err := LoadGockItems("testdata/gock.json")
	if err != nil {
		panic(err)
	}
	exg := getBinance(nil)
	gock.InterceptClient(exg.HttpClient)

	symbol := "ETH/USDT:USDT"
	jobs := [][2]string{{symbol, "1m"}}
	out, err_ := exg.WatchOhlcvs(jobs, nil)
	if err_ != nil {
		panic(err_)
	}
	count := 0
mainFor:
	for {
		select {
		case k, ok := <-out:
			if !ok {
				log.Info("read out chan fail, break")
				break mainFor
			}
			count += 1
			if count == 10 {
				err2 := exg.UnWatchOhlcvs(jobs, nil)
				if err2 != nil {
					log.Error("unwatch fail", zap.Error(err))
				} else {
					log.Info("unwatch jobs..")
				}
			}
			log.Info("ohlcv", zap.Int64("t", k.Time),
				zap.Float64("o", k.Open),
				zap.Float64("h", k.High),
				zap.Float64("l", k.Low),
				zap.Float64("c", k.Close),
				zap.Float64("v", k.Volume),
			)
		}
	}
}

func TestWatchBalance(t *testing.T) {
	exg := getBinance(nil)
	//exg.MarketType = banexg.MarketLinear
	out, err := exg.WatchBalance(nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("start watching balances")
mainFor:
	for {
		select {
		case b, ok := <-out:
			if !ok {
				log.Info("read out chan fail, break")
				break mainFor
			}
			builder := strings.Builder{}
			builder.WriteString("time:" + strconv.FormatInt(b.TimeStamp, 10) + "\n")
			for _, item := range b.Assets {
				builder.WriteString(item.Code + "\t\t")
				builder.WriteString(fmt.Sprintf("free: %f total: %f\n", item.Free, item.Total))
			}
			fmt.Print(builder.String())
		}
	}
}
