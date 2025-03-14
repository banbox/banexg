package binance

import (
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/bntp"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/h2non/gock"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWatchOHLCVs(t *testing.T) {
	testWatchOHLCVs(t, false)
}

func testWatchOHLCVs(t *testing.T, isFake bool) {
	if isFake {
		gock.DisableNetworking()
		err := LoadGockItems("testdata/gock.json")
		if err != nil {
			panic(err)
		}
	}
	exg := getBinance(map[string]interface{}{
		banexg.OptDebugWs: true,
	})
	if isFake {
		gock.InterceptClient(exg.HttpClient)
	}

	var err *errs.Error
	jobs := [][2]string{
		{"ETH/USDT", "1s"},
	}
	out, err_ := exg.WatchOHLCVs(jobs, nil)
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
				err2 := exg.UnWatchOHLCVs(jobs, nil)
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
	exg.MarketType = banexg.MarketLinear
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
	exg.MarketType = banexg.MarketLinear
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
	exg.MarketType = banexg.MarketLinear
	symbols := []string{"BTC/USDT:USDT", "ETH/USDT:USDT"}
	out, err := exg.WatchMarkPrices(symbols, map[string]interface{}{
		banexg.ParamInterval: "1s",
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
			timeStr := bntp.Now().Format("2006-01-02 15:04:05")
			builder := strings.Builder{}
			builder.WriteString("============== " + timeStr + " ===============\n")
			for symbol, price := range data {
				builder.WriteString(fmt.Sprintf("%s: %v\n", symbol, price))
			}
			fmt.Print(builder.String())
		}
	}
}

func TestWsDump(t *testing.T) {
	exg := getBinance(map[string]interface{}{
		banexg.OptDumpPath: getWsDumpPath(),
	})
	exg.MarketType = banexg.MarketLinear
	symbols := []string{"BTC/USDT:USDT", "ETH/USDT:USDT"}
	_, err := exg.WatchOrderBooks(symbols, 500, nil)
	if err != nil {
		panic(err)
	}
	log.Info("watch order books ...")
	time.AfterFunc(time.Second*30, func() {
		_, err = exg.WatchMarkPrices(symbols, nil)
		if err != nil {
			panic(err)
		}
		log.Info("watch mark prices ...")
	})
	time.Sleep(time.Second * 60)
	exg.Close()
}

func TestWsReplay(t *testing.T) {
	exg := getBinance(map[string]interface{}{
		banexg.OptReplayPath: getWsDumpPath(),
	})
	exg.MarketType = banexg.MarketLinear
	exg.SetOnWsChan(func(key string, out interface{}) {
		offset := strings.LastIndex(key, "@")
		method := key[offset+1:]
		if method == "depth" {
			chl := out.(chan *banexg.OrderBook)
			go func() {
				count := 0
				for range chl {
					count += 1
				}
				log.Info("got depth msg", zap.Int("num", count))
			}()
		} else if method == "markPrice" {
			chl := out.(chan map[string]float64)
			go func() {
				count := 0
				for range chl {
					count += 1
				}
				log.Info("got markPrice msg", zap.Int("num", count))
			}()
		} else {
			log.Info("got unknown ws chan", zap.String("key", key))
		}
	})
	err := exg.ReplayAll()
	if err != nil {
		panic(err)
	}
	exg.Close()
	time.Sleep(time.Second)
}

func getWsDumpPath() string {
	cacheDir, err_ := os.UserCacheDir()
	if err_ != nil {
		panic(err_)
	}
	return filepath.Join(cacheDir, "ban_ws_dump.gz")
}

func TestDumpCompress(t *testing.T) {
	inPath := getWsDumpPath()
	file, err := os.Open(inPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		panic(err)
	}
	defer reader.Close()
	decoder := gob.NewDecoder(reader)

	out, err := os.OpenFile(inPath+"1", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	defer out.Close()
	writer := gzip.NewWriter(out)
	defer writer.Close()
	encoder := gob.NewEncoder(writer)

	for {
		cache := make([]*banexg.WsLog, 0, 1000)
		if err_ := decoder.Decode(&cache); err_ != nil {
			// read done
			break
		}
		err = encoder.Encode(cache)
		if err != nil {
			panic(err)
		}
	}
}
