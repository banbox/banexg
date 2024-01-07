package binance

import (
	"fmt"
	"github.com/banbox/banexg/base"
	"github.com/banbox/banexg/log"
	"github.com/h2non/gock"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"testing"
	"time"
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
	exg.MarketType = base.MarketLinear
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

func TestWatchPositions(t *testing.T) {
	exg := getBinance(nil)
	exg.MarketType = base.MarketLinear
	out, err := exg.WatchPositions(nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("start watching positions")
mainFor:
	for {
		select {
		case positions, ok := <-out:
			if !ok {
				log.Info("read out chan fail, break")
				break mainFor
			}
			builder := strings.Builder{}
			builder.WriteString("=============================\n")
			for _, pos := range positions {
				builder.WriteString(pos.Symbol + ", ")
				builder.WriteString(fmt.Sprintf("%v, ", pos.Contracts))
				builder.WriteString(fmt.Sprintf("%v, ", pos.UnrealizedPnl))
				builder.WriteString("\n")
			}
			fmt.Print(builder.String())
		}
	}
}

func TestWatchMarkPrices(t *testing.T) {
	exg := getBinance(nil)
	exg.MarketType = base.MarketLinear
	symbols := []string{"BTC/USDT:USDT", "ETH/USDT:USDT"}
	out, err := exg.WatchMarkPrices(symbols, &map[string]interface{}{
		base.ParamInterval: "1s",
	})
	// 监听所有币种，3s更新:
	// out, err := exg.WatchMarkPrices(nil, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("start watching markPrices")
mainFor:
	for {
		select {
		case data, ok := <-out:
			if !ok {
				log.Info("read out chan fail, break")
				break mainFor
			}
			timeStr := time.Now().Format("2006-01-02 15:04:05")
			builder := strings.Builder{}
			builder.WriteString("============== " + timeStr + " ===============\n")
			for symbol, price := range data {
				builder.WriteString(fmt.Sprintf("%s: %v\n", symbol, price))
			}
			fmt.Print(builder.String())
		}
	}
}
