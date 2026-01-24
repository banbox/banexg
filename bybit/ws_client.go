package bybit

import (
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"go.uber.org/zap"
)

const (
	wsPrivate = "private"
)

type WsPendingRecon struct {
	Client *banexg.WsClient
	ConnID int
	Keys   []string
}

func makeHandleWsMsg(e *Bybit) banexg.FuncOnWsMsg {
	return func(client *banexg.WsClient, item *banexg.WsMsg) {
		if item == nil || client == nil {
			return
		}
		var base wsBaseMsg
		if err := utils.UnmarshalString(item.Text, &base, utils.JsonNumDefault); err != nil {
			log.Error("bybit ws msg unmarshal fail", zap.Error(err))
			return
		}
		if base.Op != "" {
			e.handleWsOp(client, &base)
			return
		}
		if base.Topic == "" {
			return
		}
		switch {
		case strings.HasPrefix(base.Topic, "orderbook."):
			e.handleWsOrderBook(client, &base)
		case strings.HasPrefix(base.Topic, "publicTrade."):
			e.handleWsTrades(client, &base)
		case strings.HasPrefix(base.Topic, "kline."):
			e.handleWsOHLCV(client, &base)
		case strings.HasPrefix(base.Topic, "tickers."):
			e.handleWsTickers(client, &base)
		case base.Topic == "wallet":
			e.handleWsWallet(client, &base)
		case strings.HasPrefix(base.Topic, "position"):
			e.handleWsPositions(client, &base)
		case strings.HasPrefix(base.Topic, "execution"):
			e.handleWsExecutions(client, &base)
		default:
			log.Debug("bybit ws unhandled topic", zap.String("topic", base.Topic))
		}
	}
}

func makeHandleWsReCon(e *Bybit) banexg.FuncOnWsReCon {
	return func(client *banexg.WsClient, connID int) *errs.Error {
		if client == nil {
			return nil
		}
		keys := client.GetSubKeys(connID)
		if len(keys) == 0 {
			return nil
		}
		if client.MarketType == wsPrivate {
			e.WsAuthLock.Lock()
			delete(e.WsAuthed, client.Key)
			e.WsPendingRecons[client.Key] = &WsPendingRecon{
				Client: client,
				ConnID: connID,
				Keys:   keys,
			}
			e.WsAuthLock.Unlock()
			acc, err := e.GetAccount(client.AccName)
			if err != nil {
				return err
			}
			return e.wsLoginAsync(client, acc, connID)
		}
		return e.writeWsTopics(client, connID, true, keys)
	}
}

func (e *Bybit) handleWsOp(client *banexg.WsClient, base *wsBaseMsg) {
	success, err := bybitWsOpSuccess(base)
	switch base.Op {
	case "pong", "ping":
		return
	case "auth":
		e.WsAuthLock.Lock()
		if success {
			e.WsAuthed[client.Key] = true
		}
		pending := e.WsPendingRecons[client.Key]
		if pending != nil {
			delete(e.WsPendingRecons, client.Key)
		}
		if ch, ok := e.WsAuthDone[client.Key]; ok {
			ch <- err
			delete(e.WsAuthDone, client.Key)
		}
		e.WsAuthLock.Unlock()
		if success && pending != nil {
			go e.restorePendingSubscriptions(pending)
		}
		return
	case "subscribe", "unsubscribe":
		if !success && err != nil {
			log.Warn("bybit ws subscribe error", zap.String("op", base.Op), zap.Error(err))
		}
		return
	default:
		if !success && err != nil {
			log.Warn("bybit ws op error", zap.String("op", base.Op), zap.Error(err))
		}
	}
}

func (e *Bybit) getWsPublicCategoryClient(args map[string]interface{}, symbols ...string) (string, *banexg.WsClient, *errs.Error) {
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return "", nil, err
	}
	marketType, _, err := e.LoadArgsMarketType(args, symbols...)
	if err != nil {
		return "", nil, err
	}
	// Bybit uses distinct public WS endpoints per category (spot/linear/inverse/option).
	// Mixed market types would result in ambiguous symbol IDs (e.g. spot BTCUSDT vs linear BTCUSDT)
	// and wrong subscriptions. Reject early to avoid silently returning data for a different market.
	for _, sym := range symbols {
		if sym == "" {
			continue
		}
		market, err := e.GetMarket(sym)
		if err != nil {
			return "", nil, err
		}
		if market != nil && market.Type != "" && marketType != "" && market.Type != marketType {
			return "", nil, errs.NewMsg(errs.CodeParamInvalid, "mixed market types for ws subscribe: %s type=%s want=%s", sym, market.Type, marketType)
		}
	}
	category, err := bybitCategoryFromType(marketType)
	if err != nil {
		return "", nil, err
	}
	client, err := e.getWsPublicClient(category, "")
	if err != nil {
		return "", nil, err
	}
	return category, client, nil
}

func (e *Bybit) getWsPublicClient(marketType, accName string) (*banexg.WsClient, *errs.Error) {
	var hostKey string
	switch marketType {
	case banexg.MarketSpot:
		hostKey = HostWsPublicSpot
	case banexg.MarketLinear:
		hostKey = HostWsPublicLinear
	case banexg.MarketInverse:
		hostKey = HostWsPublicInverse
	case banexg.MarketOption:
		hostKey = HostWsPublicOption
	default:
		return nil, errs.NewMsg(errs.CodeParamInvalid, "unsupported ws market: %s", marketType)
	}
	wsUrl := e.GetHost(hostKey)
	if wsUrl == "" {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "ws host missing for %s", marketType)
	}
	return e.GetClient(wsUrl, marketType, accName)
}

func (e *Bybit) getWsPrivateClient(accName string) (*banexg.WsClient, *errs.Error) {
	wsUrl := e.GetHost(HostWsPrivate)
	if wsUrl == "" {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "ws host missing for private")
	}
	return e.GetClient(wsUrl, wsPrivate, accName)
}

func (e *Bybit) getAuthClient(params map[string]interface{}) (*banexg.WsClient, *errs.Error) {
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, err
	}
	acc, err := e.GetAccount(e.GetAccName(params))
	if err != nil {
		return nil, err
	}
	client, err := e.getWsPrivateClient(acc.Name)
	if err != nil {
		return nil, err
	}
	if err := e.wsLogin(client, acc, 0); err != nil {
		return nil, err
	}
	return client, nil
}

func (e *Bybit) wsLoginAsync(client *banexg.WsClient, acc *banexg.Account, connID int) *errs.Error {
	if client == nil || acc == nil {
		return errs.NewMsg(errs.CodeParamInvalid, "invalid ws login args")
	}
	_, creds, err := e.GetAccountCreds(acc.Name)
	if err != nil {
		return err
	}
	expires := time.Now().UnixMilli() + 10000
	payload := "GET/realtime" + strconv.FormatInt(expires, 10)
	sign, err2 := utils.Signature(payload, creds.Secret, "hmac", "sha256", "hex")
	if err2 != nil {
		return errs.New(errs.CodeSignFail, err2)
	}
	req := map[string]interface{}{
		"op":   "auth",
		"args": []interface{}{creds.ApiKey, expires, sign},
	}
	_, conn := client.UpdateSubs(connID, true, []string{})
	if conn == nil {
		return errs.NewMsg(errs.CodeRunTime, "get ws conn fail")
	}
	return client.Write(conn, req, nil)
}

func (e *Bybit) wsLogin(client *banexg.WsClient, acc *banexg.Account, connID int) *errs.Error {
	if client == nil || acc == nil {
		return errs.NewMsg(errs.CodeParamInvalid, "invalid ws login args")
	}
	e.WsAuthLock.Lock()
	if e.WsAuthed[client.Key] {
		e.WsAuthLock.Unlock()
		return nil
	}
	if doneCh, waiting := e.WsAuthDone[client.Key]; waiting {
		e.WsAuthLock.Unlock()
		select {
		case authErr := <-doneCh:
			select {
			case doneCh <- authErr:
			default:
			}
			return authErr
		case <-time.After(10 * time.Second):
			return errs.NewMsg(errs.CodeTimeout, "ws login timeout (waiting)")
		}
	}
	doneCh := make(chan *errs.Error, 10)
	e.WsAuthDone[client.Key] = doneCh
	e.WsAuthLock.Unlock()
	if err := e.wsLoginAsync(client, acc, connID); err != nil {
		e.WsAuthLock.Lock()
		delete(e.WsAuthDone, client.Key)
		e.WsAuthLock.Unlock()
		return err
	}
	select {
	case authErr := <-doneCh:
		if authErr == nil {
			e.WsAuthLock.Lock()
			e.WsAuthed[client.Key] = true
			e.WsAuthLock.Unlock()
		}
		return authErr
	case <-time.After(10 * time.Second):
		e.WsAuthLock.Lock()
		delete(e.WsAuthDone, client.Key)
		e.WsAuthLock.Unlock()
		return errs.NewMsg(errs.CodeTimeout, "ws login timeout")
	}
}

func (e *Bybit) restorePendingSubscriptions(recon *WsPendingRecon) {
	if recon == nil || recon.Client == nil || len(recon.Keys) == 0 {
		return
	}
	if err := e.writeWsTopics(recon.Client, recon.ConnID, true, recon.Keys); err != nil {
		log.Error("restore bybit ws subscriptions failed", zap.Error(err))
	}
}

func (e *Bybit) writeWsTopics(client *banexg.WsClient, connID int, isSub bool, keys []string) *errs.Error {
	if client == nil {
		return errs.NewMsg(errs.CodeParamInvalid, "ws client required")
	}
	if len(keys) == 0 {
		return nil
	}
	if !isSub {
		return e.writeWsUnsub(client, keys)
	}
	batchSize := bybitWsBatchSize(client.MarketType)
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[i:end]
		_, conn := client.UpdateSubs(connID, true, batch)
		if conn == nil {
			return errs.NewMsg(errs.CodeRunTime, "get ws conn fail")
		}
		req := map[string]interface{}{
			"op":   "subscribe",
			"args": batch,
		}
		if err := client.Write(conn, req, nil); err != nil {
			return err
		}
	}
	return nil
}

func (e *Bybit) writeWsUnsub(client *banexg.WsClient, keys []string) *errs.Error {
	connMap, lock := client.LockConns()
	connDups := maps.Clone(connMap)
	lock.Unlock()
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}
	for connID, conn := range connDups {
		subKeys := client.GetSubKeys(connID)
		if len(subKeys) == 0 {
			continue
		}
		useKeys := make([]string, 0, len(subKeys))
		for _, key := range subKeys {
			if _, ok := keySet[key]; ok {
				useKeys = append(useKeys, key)
			}
		}
		if len(useKeys) == 0 {
			continue
		}
		_, _ = client.UpdateSubs(connID, false, useKeys)
		req := map[string]interface{}{
			"op":   "unsubscribe",
			"args": useKeys,
		}
		if err := client.Write(conn, req, nil); err != nil {
			return err
		}
	}
	return nil
}

func bybitWsSymbolsFromJobs(jobs [][2]string) []string {
	symbols := make([]string, 0, len(jobs))
	for _, job := range jobs {
		symbols = append(symbols, job[0])
	}
	return symbols
}

func watchBybitWsPublicSymbols[T any](
	e *Bybit,
	args map[string]interface{},
	symbols []string,
	topicFn func(*Bybit, []string) ([]string, *errs.Error),
	chanPrefix string,
	dumpName string,
	refKeys []string,
	create func(int) chan T,
) (chan T, *errs.Error) {
	_, client, err := e.getWsPublicCategoryClient(args, symbols...)
	if err != nil {
		return nil, err
	}
	keys, err := topicFn(e, symbols)
	if err != nil {
		return nil, err
	}
	if err := e.writeWsTopics(client, 0, true, keys); err != nil {
		return nil, err
	}
	chanKey := client.Prefix(chanPrefix)
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, refKeys...)
	e.DumpWS(dumpName, symbols)
	return out, nil
}

func (e *Bybit) unwatchWsPublicSymbols(
	args map[string]interface{},
	symbols []string,
	topicFn func(*Bybit, []string) ([]string, *errs.Error),
	chanPrefix string,
	refKeys []string,
) *errs.Error {
	_, client, err := e.getWsPublicCategoryClient(args, symbols...)
	if err != nil {
		return err
	}
	keys, err := topicFn(e, symbols)
	if err != nil {
		return err
	}
	if err := e.writeWsTopics(client, 0, false, keys); err != nil {
		return err
	}
	chanKey := client.Prefix(chanPrefix)
	e.DelWsChanRefs(chanKey, refKeys...)
	return nil
}

func watchBybitWsPublicJobs[T any](
	e *Bybit,
	args map[string]interface{},
	jobs [][2]string,
	topicFn func(*Bybit, [][2]string) ([]string, []string, *errs.Error),
	chanPrefix string,
	dumpName string,
	create func(int) chan T,
) (chan T, *errs.Error) {
	symbols := bybitWsSymbolsFromJobs(jobs)
	_, client, err := e.getWsPublicCategoryClient(args, symbols...)
	if err != nil {
		return nil, err
	}
	keys, refKeys, err := topicFn(e, jobs)
	if err != nil {
		return nil, err
	}
	if err := e.writeWsTopics(client, 0, true, keys); err != nil {
		return nil, err
	}
	chanKey := client.Prefix(chanPrefix)
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, refKeys...)
	e.DumpWS(dumpName, jobs)
	return out, nil
}

func (e *Bybit) unwatchWsPublicJobs(
	args map[string]interface{},
	jobs [][2]string,
	topicFn func(*Bybit, [][2]string) ([]string, []string, *errs.Error),
	chanPrefix string,
) *errs.Error {
	symbols := bybitWsSymbolsFromJobs(jobs)
	_, client, err := e.getWsPublicCategoryClient(args, symbols...)
	if err != nil {
		return err
	}
	keys, refKeys, err := topicFn(e, jobs)
	if err != nil {
		return err
	}
	if err := e.writeWsTopics(client, 0, false, keys); err != nil {
		return err
	}
	chanKey := client.Prefix(chanPrefix)
	e.DelWsChanRefs(chanKey, refKeys...)
	return nil
}

func bybitWsPrivatePositionTopic(args map[string]interface{}, opName string) (string, *errs.Error) {
	topic := "position"
	if marketType := utils.GetMapVal(args, banexg.ParamMarket, ""); marketType != "" {
		// docs/bybit_v5/websocket/private/position.md:
		// Categorised topics: position.linear / position.inverse / position.option
		// Spot/margin positions are not supported.
		if cat, err := bybitCategoryFromType(marketType); err == nil {
			if cat == banexg.MarketSpot {
				return "", errs.NewMsg(errs.CodeUnsupportMarket, "%s supports linear/inverse/option only", opName)
			}
			topic = "position." + cat
		}
	}
	return topic, nil
}

func watchBybitWsPrivateTopic[T any](
	e *Bybit,
	args map[string]interface{},
	topic string,
	chanPrefix string,
	dumpName string,
	refKeys []string,
	dumpData interface{},
	create func(int) chan T,
) (*banexg.WsClient, chan T, *errs.Error) {
	client, err := e.getAuthClient(args)
	if err != nil {
		return nil, nil, err
	}
	if err := e.writeWsTopics(client, 0, true, []string{topic}); err != nil {
		return nil, nil, err
	}
	chanKey := client.Prefix(chanPrefix)
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, refKeys...)
	e.DumpWS(dumpName, dumpData)
	return client, out, nil
}

/*
makeCheckWsTimeout creates a goroutine that:
1. Periodically sends "ping" to all Bybit WebSocket connections to keep them alive.
*/
func makeCheckWsTimeout(e *Bybit) func() {
	return func() {
		e.WsChecking = true
		defer func() {
			e.WsChecking = false
		}()
		pingInterval := time.Second * 20
		for {
			time.Sleep(pingInterval)
			for _, client := range e.WSClients {
				conns, lock := client.LockConns()
				for _, conn := range conns {
					if err := client.Write(conn, map[string]interface{}{"op": "ping"}, nil); err != nil {
						log.Warn("send bybit ws ping fail", zap.String("url", client.URL),
							zap.Int("conn", conn.GetID()), zap.Error(err))
					}
				}
				lock.Unlock()
			}
		}
	}
}
