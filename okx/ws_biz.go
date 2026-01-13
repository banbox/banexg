package okx

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	"github.com/banbox/banexg/log"
	"github.com/banbox/banexg/utils"
	"github.com/banbox/bntp"
	"go.uber.org/zap"
)

const (
	wsPublic   = "public"
	wsPrivate  = "private"
	wsBusiness = "business"
)

func makeHandleWsMsg(e *OKX) banexg.FuncOnWsMsg {
	return func(client *banexg.WsClient, item *banexg.WsMsg) {
		if item == nil {
			return
		}
		var msg map[string]interface{}
		if err := utils.UnmarshalString(item.Text, &msg, utils.JsonNumAuto); err != nil {
			log.Error("ws msg unmarshal fail", zap.Error(err))
			return
		}
		if event, ok := msg["event"].(string); ok && event != "" {
			// Handle login success/error events
			if event == "login" {
				code := getMapString(msg, "code")
				e.WsAuthLock.Lock()
				loginSuccess := code == "0" || code == ""
				if loginSuccess {
					e.WsAuthed[client.Key] = true
				}
				// Check for pending reconnection subscriptions
				pendingRecon := e.WsPendingRecons[client.Key]
				if pendingRecon != nil {
					delete(e.WsPendingRecons, client.Key)
				}
				// Notify waiting goroutines if any
				if ch, ok := e.WsAuthDone[client.Key]; ok {
					if loginSuccess {
						ch <- nil
					} else {
						errMsg := getMapString(msg, "msg")
						ch <- errs.NewMsg(errs.CodeUnauthorized, "ws login failed: %s - %s", code, errMsg)
					}
					delete(e.WsAuthDone, client.Key)
				}
				e.WsAuthLock.Unlock()
				// Restore subscriptions after successful reconnection login
				if loginSuccess && pendingRecon != nil && len(pendingRecon.Keys) > 0 {
					go e.restorePendingSubscriptions(pendingRecon)
				}
				return
			}
			if event == "error" {
				log.Error("ws event error", zap.String("msg", item.Text))
				// Check if this is a login-related error
				code := getMapString(msg, "code")
				if code == "60011" || code == "60009" || code == "60012" {
					// Login required or login failed errors
					e.WsAuthLock.Lock()
					if ch, ok := e.WsAuthDone[client.Key]; ok {
						errMsg := getMapString(msg, "msg")
						ch <- errs.NewMsg(errs.CodeUnauthorized, "ws auth error: %s - %s", code, errMsg)
						delete(e.WsAuthDone, client.Key)
					}
					e.WsAuthLock.Unlock()
				}
			}
			return
		}
		arg, _ := msg["arg"].(map[string]interface{})
		channel := getMapString(arg, "channel")
		switch {
		case channel == WsChanTrades:
			e.handleWsTrades(client, msg, arg)
		case channel == WsChanBooks || channel == WsChanBooks5 || channel == "bbo-tbt" || channel == "books-l2-tbt" || channel == "books50-l2-tbt":
			e.handleWsOrderBooks(client, msg, arg, channel)
		case channel == WsChanBalancePosition:
			e.handleWsBalanceAndPosition(client, msg)
		case channel == WsChanOrders:
			e.handleWsOrders(client, msg, arg)
		case channel == WsChanMarkPrice:
			e.handleWsMarkPrices(client, msg, arg)
		case strings.HasPrefix(channel, WsChanCandlePrefix):
			e.handleWsOHLCV(client, msg, arg)
		default:
			if channel != "" {
				log.Debug("unhandled ws channel", zap.String("channel", channel))
			}
		}
	}
}

func makeHandleWsReCon(e *OKX) banexg.FuncOnWsReCon {
	return func(client *banexg.WsClient, connID int) *errs.Error {
		if client == nil {
			return nil
		}
		keys := client.GetSubKeys(connID)
		if client.MarketType == wsPrivate {
			// Clear auth state on reconnect to force re-login
			e.WsAuthLock.Lock()
			delete(e.WsAuthed, client.Key)
			// Store pending recon info for subscription restoration after login
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
			// Send login request without waiting (non-blocking)
			// Subscriptions will be restored in message handler after login succeeds
			return e.wsLoginAsync(client, acc, connID)
		}
		// For public WebSocket, restore subscriptions immediately
		if len(keys) == 0 {
			return nil
		}
		args := make([]map[string]interface{}, 0, len(keys))
		for _, key := range keys {
			ch, instType, instId := parseWsKey(key)
			arg := map[string]interface{}{FldChannel: ch}
			if instType != "" {
				arg[FldInstType] = instType
			}
			if instId != "" {
				arg[FldInstId] = instId
			}
			args = append(args, arg)
		}
		return e.writeWsArgs(client, connID, true, keys, args)
	}
}

func (e *OKX) WatchOrderBooks(symbols []string, limit int, params map[string]interface{}) (chan *banexg.OrderBook, *errs.Error) {
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for WatchOrderBooks")
	}
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, err
	}
	client, err := e.getWsClient(wsPublic, "")
	if err != nil {
		return nil, err
	}
	channel := WsChanBooks
	if limit > 0 && limit <= 5 {
		channel = WsChanBooks5
	}
	argsList := make([]map[string]interface{}, 0, len(symbols))
	keys := make([]string, 0, len(symbols))
	limits, lock := client.LockOdBookLimits()
	for _, sym := range symbols {
		id, err := e.GetMarketID(sym)
		if err != nil {
			lock.Unlock()
			return nil, err
		}
		argsList = append(argsList, map[string]interface{}{FldChannel: channel, FldInstId: id})
		keys = append(keys, buildWsKey(channel, id))
		if limit > 0 {
			limits[sym] = limit
		} else if limits[sym] == 0 {
			limits[sym] = 400
		}
	}
	lock.Unlock()
	if err := e.writeWsArgs(client, 0, true, keys, argsList); err != nil {
		return nil, err
	}
	chanKey := client.Prefix(channel)
	create := func(cap int) chan *banexg.OrderBook { return make(chan *banexg.OrderBook, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, params)
	e.AddWsChanRefs(chanKey, symbols...)
	e.DumpWS("WatchOrderBooks", symbols)
	return out, nil
}

func (e *OKX) UnWatchOrderBooks(symbols []string, params map[string]interface{}) *errs.Error {
	if len(symbols) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchOrderBooks")
	}
	client, err := e.getWsClient(wsPublic, "")
	if err != nil {
		return err
	}
	channel := WsChanBooks
	argsList := make([]map[string]interface{}, 0, len(symbols))
	keys := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		id, err := e.GetMarketID(sym)
		if err != nil {
			return err
		}
		argsList = append(argsList, map[string]interface{}{FldChannel: channel, FldInstId: id})
		keys = append(keys, buildWsKey(channel, id))
	}
	if err := e.writeWsArgs(client, 0, false, keys, argsList); err != nil {
		return err
	}
	chanKey := client.Prefix(channel)
	e.DelWsChanRefs(chanKey, symbols...)
	return nil
}

func (e *OKX) WatchTrades(symbols []string, params map[string]interface{}) (chan *banexg.Trade, *errs.Error) {
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for WatchTrades")
	}
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, err
	}
	client, err := e.getWsClient(wsPublic, "")
	if err != nil {
		return nil, err
	}
	channel := WsChanTrades
	argsList := make([]map[string]interface{}, 0, len(symbols))
	keys := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		id, err := e.GetMarketID(sym)
		if err != nil {
			return nil, err
		}
		argsList = append(argsList, map[string]interface{}{FldChannel: channel, FldInstId: id})
		keys = append(keys, buildWsKey(channel, id))
	}
	if err := e.writeWsArgs(client, 0, true, keys, argsList); err != nil {
		return nil, err
	}
	chanKey := client.Prefix(channel)
	create := func(cap int) chan *banexg.Trade { return make(chan *banexg.Trade, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, params)
	e.AddWsChanRefs(chanKey, symbols...)
	e.DumpWS("WatchTrades", symbols)
	return out, nil
}

func (e *OKX) UnWatchTrades(symbols []string, params map[string]interface{}) *errs.Error {
	if len(symbols) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchTrades")
	}
	client, err := e.getWsClient(wsPublic, "")
	if err != nil {
		return err
	}
	channel := WsChanTrades
	argsList := make([]map[string]interface{}, 0, len(symbols))
	keys := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		id, err := e.GetMarketID(sym)
		if err != nil {
			return err
		}
		argsList = append(argsList, map[string]interface{}{FldChannel: channel, FldInstId: id})
		keys = append(keys, buildWsKey(channel, id))
	}
	if err := e.writeWsArgs(client, 0, false, keys, argsList); err != nil {
		return err
	}
	chanKey := client.Prefix(channel)
	e.DelWsChanRefs(chanKey, symbols...)
	return nil
}

func (e *OKX) WatchOHLCVs(jobs [][2]string, params map[string]interface{}) (chan *banexg.PairTFKline, *errs.Error) {
	if len(jobs) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "jobs required for WatchOHLCVs")
	}
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, err
	}
	client, err := e.getWsClient(wsBusiness, "")
	if err != nil {
		return nil, err
	}
	argsList := make([]map[string]interface{}, 0, len(jobs))
	keys := make([]string, 0, len(jobs))
	refKeys := make([]string, 0, len(jobs))
	for _, job := range jobs {
		symbol := job[0]
		timeframe := job[1]
		if symbol == "" || timeframe == "" {
			return nil, errs.NewMsg(errs.CodeParamInvalid, "invalid job for WatchOHLCVs")
		}
		id, err := e.GetMarketID(symbol)
		if err != nil {
			return nil, err
		}
		tf := e.GetTimeFrame(timeframe)
		if tf == "" {
			return nil, errs.NewMsg(errs.CodeInvalidTimeFrame, "invalid timeframe: %s", timeframe)
		}
		channel := okxCandleChannel(tf)
		argsList = append(argsList, map[string]interface{}{FldChannel: channel, FldInstId: id})
		keys = append(keys, buildWsKey(channel, id))
		refKeys = append(refKeys, symbol+"@"+timeframe)
	}
	if err := e.writeWsArgs(client, 0, true, keys, argsList); err != nil {
		return nil, err
	}
	chanKey := client.Prefix("candle")
	create := func(cap int) chan *banexg.PairTFKline { return make(chan *banexg.PairTFKline, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, params)
	e.AddWsChanRefs(chanKey, refKeys...)
	e.DumpWS("WatchOHLCVs", jobs)
	return out, nil
}

func (e *OKX) UnWatchOHLCVs(jobs [][2]string, params map[string]interface{}) *errs.Error {
	if len(jobs) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "jobs required for UnWatchOHLCVs")
	}
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return err
	}
	client, err := e.getWsClient(wsBusiness, "")
	if err != nil {
		return err
	}
	argsList := make([]map[string]interface{}, 0, len(jobs))
	keys := make([]string, 0, len(jobs))
	refKeys := make([]string, 0, len(jobs))
	for _, job := range jobs {
		symbol := job[0]
		timeframe := job[1]
		if symbol == "" || timeframe == "" {
			return errs.NewMsg(errs.CodeParamInvalid, "invalid job for UnWatchOHLCVs")
		}
		id, err := e.GetMarketID(symbol)
		if err != nil {
			return err
		}
		tf := e.GetTimeFrame(timeframe)
		if tf == "" {
			return errs.NewMsg(errs.CodeInvalidTimeFrame, "invalid timeframe: %s", timeframe)
		}
		channel := okxCandleChannel(tf)
		argsList = append(argsList, map[string]interface{}{FldChannel: channel, FldInstId: id})
		keys = append(keys, buildWsKey(channel, id))
		refKeys = append(refKeys, symbol+"@"+timeframe)
	}
	if err := e.writeWsArgs(client, 0, false, keys, argsList); err != nil {
		return err
	}
	chanKey := client.Prefix("candle")
	e.DelWsChanRefs(chanKey, refKeys...)
	return nil
}

func (e *OKX) WatchMarkPrices(symbols []string, params map[string]interface{}) (chan map[string]float64, *errs.Error) {
	if len(symbols) == 0 {
		return nil, errs.NewMsg(errs.CodeParamRequired, "symbols required for WatchMarkPrices")
	}
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, err
	}
	client, err := e.getWsClient(wsPublic, "")
	if err != nil {
		return nil, err
	}
	channel := WsChanMarkPrice
	argsList := make([]map[string]interface{}, 0, len(symbols))
	keys := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		id, err := e.GetMarketID(sym)
		if err != nil {
			return nil, err
		}
		argsList = append(argsList, map[string]interface{}{FldChannel: channel, FldInstId: id})
		keys = append(keys, buildWsKey(channel, id))
	}
	if err := e.writeWsArgs(client, 0, true, keys, argsList); err != nil {
		return nil, err
	}
	chanKey := client.Prefix("markPrice")
	create := func(cap int) chan map[string]float64 { return make(chan map[string]float64, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, params)
	e.AddWsChanRefs(chanKey, "markPrice")
	e.DumpWS("WatchMarkPrices", symbols)
	return out, nil
}

func (e *OKX) UnWatchMarkPrices(symbols []string, params map[string]interface{}) *errs.Error {
	if len(symbols) == 0 {
		return errs.NewMsg(errs.CodeParamRequired, "symbols required for UnWatchMarkPrices")
	}
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return err
	}
	client, err := e.getWsClient(wsPublic, "")
	if err != nil {
		return err
	}
	channel := WsChanMarkPrice
	argsList := make([]map[string]interface{}, 0, len(symbols))
	keys := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		id, err := e.GetMarketID(sym)
		if err != nil {
			return err
		}
		argsList = append(argsList, map[string]interface{}{FldChannel: channel, FldInstId: id})
		keys = append(keys, buildWsKey(channel, id))
	}
	if err := e.writeWsArgs(client, 0, false, keys, argsList); err != nil {
		return err
	}
	chanKey := client.Prefix("markPrice")
	e.DelWsChanRefs(chanKey, "markPrice")
	return nil
}

func (e *OKX) WatchBalance(params map[string]interface{}) (chan *banexg.Balances, *errs.Error) {
	client, err := e.subscribePrivateChannel(params, WsChanBalancePosition, "", "")
	if err != nil {
		return nil, err
	}
	chanKey := client.Prefix("balance")
	create := func(cap int) chan *banexg.Balances { return make(chan *banexg.Balances, cap) }
	args := utils.SafeParams(params)
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	if balances, err := e.FetchBalance(args); err == nil && balances != nil {
		if acc, err := e.GetAccount(client.AccName); err == nil {
			acc.LockBalance.Lock()
			acc.MarBalances[client.MarketType] = balances
			acc.LockBalance.Unlock()
		}
		out <- balances
	}
	e.DumpWS("WatchBalance", nil)
	return out, nil
}

func (e *OKX) WatchPositions(params map[string]interface{}) (chan []*banexg.Position, *errs.Error) {
	client, err := e.subscribePrivateChannel(params, WsChanBalancePosition, "", "")
	if err != nil {
		return nil, err
	}
	chanKey := client.Prefix("positions")
	create := func(cap int) chan []*banexg.Position { return make(chan []*banexg.Position, cap) }
	args := utils.SafeParams(params)
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	if positions, err := e.FetchPositions(nil, args); err == nil && len(positions) > 0 {
		if acc, err := e.GetAccount(client.AccName); err == nil {
			acc.LockPos.Lock()
			acc.MarPositions[client.MarketType] = positions
			acc.LockPos.Unlock()
		}
		out <- positions
	}
	e.DumpWS("WatchPositions", nil)
	return out, nil
}

func (e *OKX) WatchAccountConfig(params map[string]interface{}) (chan *banexg.AccountConfig, *errs.Error) {
	client, err := e.subscribePrivateChannel(params, WsChanBalancePosition, "", "")
	if err != nil {
		return nil, err
	}
	chanKey := client.Prefix("accConfig")
	create := func(cap int) chan *banexg.AccountConfig { return make(chan *banexg.AccountConfig, cap) }
	args := utils.SafeParams(params)
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	e.DumpWS("WatchAccountConfig", nil)
	return out, nil
}

func (e *OKX) WatchMyTrades(params map[string]interface{}) (chan *banexg.MyTrade, *errs.Error) {
	args := utils.SafeParams(params)
	symbol := utils.PopMapVal(args, banexg.ParamSymbol, "")
	instType := ""
	instId := ""
	if symbol != "" {
		_, market, err := e.LoadArgsMarket(symbol, args)
		if err != nil {
			return nil, err
		}
		instId = market.ID
		instType = instTypeFromMarket(market)
	} else {
		marketType, contractType, err := e.LoadArgsMarketType(args, "")
		if err != nil {
			return nil, err
		}
		instType = instTypeByMarket(marketType, contractType)
	}
	client, err := e.subscribePrivateChannel(args, WsChanOrders, instType, instId)
	if err != nil {
		return nil, err
	}
	chanKey := client.Prefix("mytrades")
	create := func(cap int) chan *banexg.MyTrade { return make(chan *banexg.MyTrade, cap) }
	out := banexg.GetWsOutChan(e.Exchange, chanKey, create, args)
	e.AddWsChanRefs(chanKey, "account")
	e.DumpWS("WatchMyTrades", nil)
	return out, nil
}

func (e *OKX) getWsClient(kind, accName string) (*banexg.WsClient, *errs.Error) {
	var hostKey string
	switch kind {
	case wsPublic:
		hostKey = HostWsPublic
	case wsPrivate:
		hostKey = HostWsPrivate
	case wsBusiness:
		hostKey = HostWsBusiness
	default:
		return nil, errs.NewMsg(errs.CodeParamInvalid, "invalid ws type: %s", kind)
	}
	wsUrl := e.GetHost(hostKey)
	if wsUrl == "" {
		return nil, errs.NewMsg(errs.CodeParamInvalid, "ws host missing for %s", kind)
	}
	return e.GetClient(wsUrl, kind, accName)
}

func (e *OKX) subscribePrivateChannel(params map[string]interface{}, channel, instType, instId string) (*banexg.WsClient, *errs.Error) {
	client, err := e.getAuthClient(params)
	if err != nil {
		return nil, err
	}
	arg := map[string]interface{}{FldChannel: channel}
	if instType != "" {
		arg[FldInstType] = instType
	}
	if instId != "" {
		arg[FldInstId] = instId
	}
	key := buildWsKeyWithType(channel, instType, instId)
	if err := e.writeWsArgs(client, 0, true, []string{key}, []map[string]interface{}{arg}); err != nil {
		return nil, err
	}
	return client, nil
}

func (e *OKX) getAuthClient(params map[string]interface{}) (*banexg.WsClient, *errs.Error) {
	_, err := e.LoadMarkets(false, nil)
	if err != nil {
		return nil, err
	}
	acc, err := e.GetAccount(e.GetAccName(params))
	if err != nil {
		return nil, err
	}
	client, err := e.getWsClient(wsPrivate, acc.Name)
	if err != nil {
		return nil, err
	}
	if err := e.wsLogin(client, acc, 0); err != nil {
		return nil, err
	}
	return client, nil
}

// wsLoginAsync sends login request without waiting for response (for reconnection).
func (e *OKX) wsLoginAsync(client *banexg.WsClient, acc *banexg.Account, connID int) *errs.Error {
	if client == nil || acc == nil {
		return errs.NewMsg(errs.CodeParamInvalid, "invalid ws login args")
	}
	_, creds, err := e.GetAccountCreds(acc.Name)
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	payload := timestamp + "GET" + "/users/self/verify"
	sign, err2 := utils.Signature(payload, creds.Secret, "hmac", "sha256", "base64")
	if err2 != nil {
		return errs.New(errs.CodeSignFail, err2)
	}
	args := []map[string]interface{}{
		{
			"apiKey":     creds.ApiKey,
			"passphrase": creds.Password,
			"timestamp":  timestamp,
			"sign":       sign,
		},
	}
	req := map[string]interface{}{
		"op":   "login",
		"args": args,
	}
	_, conn := client.UpdateSubs(connID, true, []string{})
	if conn == nil {
		return errs.NewMsg(errs.CodeRunTime, "get ws conn fail")
	}
	return client.Write(conn, req, nil)
}

func (e *OKX) wsLogin(client *banexg.WsClient, acc *banexg.Account, connID int) *errs.Error {
	if client == nil || acc == nil {
		return errs.NewMsg(errs.CodeParamInvalid, "invalid ws login args")
	}

	e.WsAuthLock.Lock()
	// Check if already authenticated
	if e.WsAuthed[client.Key] {
		e.WsAuthLock.Unlock()
		return nil
	}
	// Check if another goroutine is already logging in
	if doneCh, waiting := e.WsAuthDone[client.Key]; waiting {
		e.WsAuthLock.Unlock()
		// Wait for the existing login to complete
		select {
		case authErr := <-doneCh:
			// Put the result back for other waiters
			select {
			case doneCh <- authErr:
			default:
			}
			return authErr
		case <-time.After(10 * time.Second):
			return errs.NewMsg(errs.CodeTimeout, "ws login timeout (waiting)")
		}
	}

	// Create completion channel for this client (buffered to allow multiple reads)
	doneCh := make(chan *errs.Error, 10)
	e.WsAuthDone[client.Key] = doneCh
	e.WsAuthLock.Unlock()

	_, creds, err := e.GetAccountCreds(acc.Name)
	if err != nil {
		e.WsAuthLock.Lock()
		delete(e.WsAuthDone, client.Key)
		e.WsAuthLock.Unlock()
		return err
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	payload := timestamp + "GET" + "/users/self/verify"
	sign, err2 := utils.Signature(payload, creds.Secret, "hmac", "sha256", "base64")
	if err2 != nil {
		e.WsAuthLock.Lock()
		delete(e.WsAuthDone, client.Key)
		e.WsAuthLock.Unlock()
		return errs.New(errs.CodeSignFail, err2)
	}
	args := []map[string]interface{}{
		{
			"apiKey":     creds.ApiKey,
			"passphrase": creds.Password,
			"timestamp":  timestamp,
			"sign":       sign,
		},
	}
	req := map[string]interface{}{
		"op":   "login",
		"args": args,
	}
	_, conn := client.UpdateSubs(connID, true, []string{})
	if conn == nil {
		e.WsAuthLock.Lock()
		delete(e.WsAuthDone, client.Key)
		e.WsAuthLock.Unlock()
		return errs.NewMsg(errs.CodeRunTime, "get ws conn fail")
	}
	if writeErr := client.Write(conn, req, nil); writeErr != nil {
		e.WsAuthLock.Lock()
		delete(e.WsAuthDone, client.Key)
		e.WsAuthLock.Unlock()
		return writeErr
	}

	// Wait for login response with timeout
	select {
	case authErr := <-doneCh:
		e.WsAuthLock.Lock()
		if authErr == nil {
			e.WsAuthed[client.Key] = true
		}
		// Keep channel for other waiters, clean up after brief delay
		e.WsAuthLock.Unlock()
		go func() {
			time.Sleep(500 * time.Millisecond)
			e.WsAuthLock.Lock()
			delete(e.WsAuthDone, client.Key)
			e.WsAuthLock.Unlock()
		}()
		return authErr
	case <-time.After(10 * time.Second):
		e.WsAuthLock.Lock()
		delete(e.WsAuthDone, client.Key)
		e.WsAuthLock.Unlock()
		return errs.NewMsg(errs.CodeTimeout, "ws login timeout")
	}
}

// restorePendingSubscriptions restores subscriptions after successful reconnection login.
func (e *OKX) restorePendingSubscriptions(recon *WsPendingRecon) {
	if recon == nil || recon.Client == nil || len(recon.Keys) == 0 {
		return
	}
	args := make([]map[string]interface{}, 0, len(recon.Keys))
	for _, key := range recon.Keys {
		ch, instType, instId := parseWsKey(key)
		arg := map[string]interface{}{FldChannel: ch}
		if instType != "" {
			arg[FldInstType] = instType
		}
		if instId != "" {
			arg[FldInstId] = instId
		}
		args = append(args, arg)
	}
	if err := e.writeWsArgs(recon.Client, recon.ConnID, true, recon.Keys, args); err != nil {
		log.Error("restore subscriptions failed", zap.Error(err))
	}
}

func (e *OKX) writeWsArgs(client *banexg.WsClient, connID int, isSub bool, keys []string, args []map[string]interface{}) *errs.Error {
	if client == nil {
		return errs.NewMsg(errs.CodeParamInvalid, "ws client required")
	}
	_, conn := client.UpdateSubs(connID, isSub, keys)
	if conn == nil {
		return errs.NewMsg(errs.CodeRunTime, "get ws conn fail")
	}
	op := "subscribe"
	if !isSub {
		op = "unsubscribe"
	}
	req := map[string]interface{}{
		"op":   op,
		"args": args,
	}
	return client.Write(conn, req, nil)
}

func (e *OKX) handleWsTrades(client *banexg.WsClient, msg map[string]interface{}, arg map[string]interface{}) {
	items := getMapSlice(msg, "data")
	instId := getMapString(arg, "instId")
	if instId == "" {
		for _, item := range items {
			instId = getMapString(item, "instId")
			break
		}
	}
	if instId != "" {
		client.SetSubsKeyStamp(buildWsKey("trades", instId), bntp.UTCStamp())
	}
	chanKey := client.Prefix("trades")
	for _, item := range items {
		trade := parseWsTradeItem(e, item)
		if trade == nil {
			continue
		}
		banexg.WriteOutChan(e.Exchange, chanKey, trade, true)
	}
}

func (e *OKX) handleWsOrderBooks(client *banexg.WsClient, msg map[string]interface{}, arg map[string]interface{}, channel string) {
	items := getMapSlice(msg, "data")
	instId := getMapString(arg, "instId")
	if instId == "" {
		for _, item := range items {
			instId = getMapString(item, "instId")
			break
		}
	}
	if instId != "" {
		client.SetSubsKeyStamp(buildWsKey(channel, instId), bntp.UTCStamp())
	}
	action := getMapString(msg, "action")
	chanKey := client.Prefix(channel)
	for _, item := range items {
		book := e.applyWsOrderBookUpdate(client, item, channel, action)
		if book == nil {
			continue
		}
		banexg.WriteOutChan(e.Exchange, chanKey, book, true)
	}
}

func (e *OKX) applyWsOrderBookUpdate(client *banexg.WsClient, item map[string]interface{}, channel, action string) *banexg.OrderBook {
	instId := getMapString(item, "instId")
	if instId == "" {
		return nil
	}
	market := getMarketByIDAny(e, instId, "")
	symbol := instId
	if market != nil {
		symbol = market.Symbol
	}
	asksRaw := getMapSlice(item, "asks")
	bidsRaw := getMapSlice(item, "bids")
	asks := parseWsBookSide(asksRaw)
	bids := parseWsBookSide(bidsRaw)
	ts := parseInt(getMapString(item, "ts"))
	e.OdBookLock.Lock()
	book, ok := e.OrderBooks[symbol]
	if !ok || action == "snapshot" {
		limit := len(asks)
		if len(bids) > limit {
			limit = len(bids)
		}
		if limit == 0 {
			limit = 400
		}
		book = &banexg.OrderBook{
			Symbol:    symbol,
			TimeStamp: ts,
			Asks:      banexg.NewOdBookSide(false, limit, asks),
			Bids:      banexg.NewOdBookSide(true, limit, bids),
			Limit:     limit,
			Cache:     make([]map[string]string, 0),
		}
		e.OrderBooks[symbol] = book
		e.OdBookLock.Unlock()
		return book
	}
	e.OdBookLock.Unlock()
	if len(asks) > 0 {
		book.Asks.Update(asks)
	}
	if len(bids) > 0 {
		book.Bids.Update(bids)
	}
	book.TimeStamp = ts
	return book
}

func (e *OKX) handleWsOHLCV(client *banexg.WsClient, msg map[string]interface{}, arg map[string]interface{}) {
	items := getMapSlice(msg, "data")
	if len(items) == 0 {
		return
	}
	channel := getMapString(arg, "channel")
	instId := getMapString(arg, "instId")
	if instId == "" {
		instId = getMapString(items[0], "instId")
	}
	if channel == "" || instId == "" {
		return
	}
	symbol := instId
	if market := getMarketByIDAny(e, instId, ""); market != nil {
		symbol = market.Symbol
	}
	tf := strings.TrimPrefix(channel, "candle")
	if tf == channel {
		tf = ""
	}
	client.SetSubsKeyStamp(buildWsKey(channel, instId), bntp.UTCStamp())
	chanKey := client.Prefix("candle")
	for _, item := range items {
		kline := parseWsCandleItem(item)
		if kline == nil {
			continue
		}
		out := &banexg.PairTFKline{
			Symbol:    symbol,
			TimeFrame: tf,
			Kline:     *kline,
		}
		banexg.WriteOutChan(e.Exchange, chanKey, out, true)
	}
}

func (e *OKX) handleWsMarkPrices(client *banexg.WsClient, msg map[string]interface{}, _ map[string]interface{}) {
	items := getMapSlice(msg, "data")
	if len(items) == 0 {
		return
	}
	result := map[string]float64{}
	e.MarkPriceLock.Lock()
	for _, item := range items {
		symbol, price, marketType, instId := parseWsMarkPriceItem(e, item)
		if symbol == "" {
			continue
		}
		if marketType == "" {
			marketType = banexg.MarketSpot
		}
		data, ok := e.MarkPrices[marketType]
		if !ok {
			data = map[string]float64{}
			e.MarkPrices[marketType] = data
		}
		data[symbol] = price
		result[symbol] = price
		if instId != "" {
			client.SetSubsKeyStamp(buildWsKey("mark-price", instId), bntp.UTCStamp())
		}
	}
	e.MarkPriceLock.Unlock()
	if len(result) > 0 {
		banexg.WriteOutChan(e.Exchange, client.Prefix("markPrice"), result, true)
	}
}

func (e *OKX) handleWsBalanceAndPosition(client *banexg.WsClient, msg map[string]interface{}) {
	items := getMapSlice(msg, "data")
	if len(items) == 0 {
		return
	}
	chanKey := client.Prefix("balance")
	posChanKey := client.Prefix("positions")
	accChanKey := client.Prefix("accConfig")
	termKey := buildWsKey("balance_and_position", "")
	for _, item := range items {
		pTime := parseInt(getMapString(item, "pTime"))
		balData := getMapSlice(item, "balData")
		if len(balData) > 0 {
			balances := parseWsBalanceData(e, balData)
			if balances != nil {
				if pTime > 0 {
					balances.TimeStamp = pTime
				}
				if acc, err := e.GetAccount(client.AccName); err == nil {
					acc.LockBalance.Lock()
					acc.MarBalances[client.MarketType] = balances
					acc.LockBalance.Unlock()
				}
				banexg.WriteOutChan(e.Exchange, chanKey, balances, true)
			}
		}
		posData := getMapSlice(item, "posData")
		if len(posData) > 0 {
			positions := parseWsPositions(e, posData)
			if len(positions) > 0 {
				var accConfigs []*banexg.AccountConfig
				if acc, err := e.GetAccount(client.AccName); err == nil {
					acc.LockPos.Lock()
					acc.MarPositions[client.MarketType] = positions
					acc.LockPos.Unlock()
					accConfigs = updateAccLeverages(acc, positions)
				}
				banexg.WriteOutChan(e.Exchange, posChanKey, positions, true)
				for _, cfg := range accConfigs {
					banexg.WriteOutChan(e.Exchange, accChanKey, cfg, true)
				}
			}
		}
		if pTime > 0 {
			client.SetSubsKeyStamp(termKey, pTime)
		} else {
			client.SetSubsKeyStamp(termKey, bntp.UTCStamp())
		}
	}
}

func parseWsTradeItem(e *OKX, item map[string]interface{}) *banexg.Trade {
	instId := getMapString(item, "instId")
	if instId == "" {
		return nil
	}
	symbol := instId
	if market := getMarketByIDAny(e, instId, ""); market != nil {
		symbol = market.Symbol
	}
	price := parseFloat(getMapString(item, "px"))
	amount := parseFloat(getMapString(item, "sz"))
	trade := &banexg.Trade{
		ID:        getMapString(item, "tradeId"),
		Symbol:    symbol,
		Price:     price,
		Amount:    amount,
		Cost:      price * amount,
		Timestamp: parseInt(getMapString(item, "ts")),
		Side:      strings.ToLower(getMapString(item, "side")),
		Info:      item,
	}
	return trade
}

func parseWsBookSide(levels []map[string]interface{}) [][2]float64 {
	if len(levels) == 0 {
		return nil
	}
	res := make([][2]float64, 0, len(levels))
	for _, lvl := range levels {
		price := parseFloat(getMapString(lvl, "0"))
		size := parseFloat(getMapString(lvl, "1"))
		if price == 0 && size == 0 {
			continue
		}
		res = append(res, [2]float64{price, size})
	}
	return res
}

func parseWsBalanceData(e *OKX, items []map[string]interface{}) *banexg.Balances {
	if len(items) == 0 {
		return nil
	}
	res := &banexg.Balances{Assets: map[string]*banexg.Asset{}}
	for _, item := range items {
		ccy := getMapString(item, "ccy")
		if ccy == "" {
			continue
		}
		code := e.SafeCurrencyCode(ccy)
		free := parseFloat(getMapString(item, "availBal"))
		if free == 0 {
			free = parseFloat(getMapString(item, "cashBal"))
		}
		used := parseFloat(getMapString(item, "frozenBal"))
		total := parseFloat(getMapString(item, "eq"))
		if total == 0 {
			total = free + used
		}
		res.Assets[code] = &banexg.Asset{Code: code, Free: free, Used: used, Total: total}
	}
	return res.Init()
}

func parseWsPositions(e *OKX, items []map[string]interface{}) []*banexg.Position {
	if len(items) == 0 {
		return nil
	}
	var arr []Position
	if err := utils.DecodeStructMap(items, &arr, "json"); err != nil {
		log.Error("ws positions decode fail", zap.Error(err))
		return nil
	}
	res := make([]*banexg.Position, 0, len(arr))
	for i := range arr {
		pos := parsePosition(e, &arr[i], items[i])
		if pos != nil {
			res = append(res, pos)
		}
	}
	return res
}

func (e *OKX) handleWsOrders(client *banexg.WsClient, msg map[string]interface{}, arg map[string]interface{}) {
	items := getMapSlice(msg, "data")
	if len(items) == 0 {
		return
	}
	instType := getMapString(arg, "instType")
	chanKey := client.Prefix("mytrades")
	for _, item := range items {
		trade := parseWsMyTrade(e, item, instType)
		if trade == nil {
			continue
		}
		subKey := buildWsKeyWithType("orders", instType, "")
		if trade.Info != nil {
			if ordInstType := getMapString(trade.Info, "instType"); ordInstType != "" {
				instType = ordInstType
				subKey = buildWsKeyWithType("orders", instType, "")
			}
			if instId := getMapString(trade.Info, "instId"); instId != "" {
				subKey = buildWsKeyWithType("orders", instType, instId)
			}
		}
		client.SetSubsKeyStamp(subKey, bntp.UTCStamp())
		banexg.WriteOutChan(e.Exchange, chanKey, trade, true)
	}
}

func parseWsMyTrade(e *OKX, item map[string]interface{}, instType string) *banexg.MyTrade {
	if item == nil {
		return nil
	}
	var ord WsOrder
	if err := utils.DecodeStructMap(item, &ord, "json"); err != nil {
		log.Error("ws order decode fail", zap.Error(err))
		return nil
	}
	if ord.InstType == "" {
		ord.InstType = instType
	}
	fillSz := parseFloat(ord.FillSz)
	if fillSz == 0 {
		return nil
	}
	price := parseFloat(ord.FillPx)
	if price == 0 {
		price = parseFloat(ord.AvgPx)
	}
	marketType := parseMarketType(ord.InstType, "")
	symbol := ord.InstId
	if marketType != "" {
		symbol = e.SafeSymbol(ord.InstId, "", marketType)
		if symbol == "" {
			symbol = ord.InstId
		}
	}
	feeCost := parseFloat(ord.FillFee)
	feeCcy := ord.FillFeeCcy
	if feeCost == 0 {
		feeCost = parseFloat(ord.Fee)
		feeCcy = ord.FeeCcy
	}
	var fee *banexg.Fee
	if feeCost != 0 || feeCcy != "" {
		fee = &banexg.Fee{Currency: feeCcy, Cost: feeCost}
	}
	orderType, _, _ := mapOrderType(ord.OrdType)
	ts := parseInt(ord.FillTime)
	if ts == 0 {
		ts = parseInt(ord.UTime)
	}
	trade := &banexg.MyTrade{
		Trade: banexg.Trade{
			ID:        ord.TradeId,
			Symbol:    symbol,
			Side:      strings.ToLower(ord.Side),
			Type:      orderType,
			Amount:    fillSz,
			Price:     price,
			Cost:      price * fillSz,
			Order:     ord.OrdId,
			Timestamp: ts,
			Fee:       fee,
			Info:      item,
		},
		Filled:     parseFloat(ord.AccFillSz),
		ClientID:   ord.ClOrdId,
		Average:    parseFloat(ord.AvgPx),
		State:      mapOrderStatus(ord.State),
		PosSide:    strings.ToLower(ord.PosSide),
		ReduceOnly: parseBoolString(ord.ReduceOnly),
		Info:       item,
	}
	return trade
}

func okxCandleChannel(timeframe string) string {
	return "candle" + timeframe
}

func parseWsCandleItem(item map[string]interface{}) *banexg.Kline {
	if item == nil {
		return nil
	}
	stamp := parseInt(getMapString(item, "0"))
	open := parseFloat(getMapString(item, "1"))
	high := parseFloat(getMapString(item, "2"))
	low := parseFloat(getMapString(item, "3"))
	closeP := parseFloat(getMapString(item, "4"))
	vol := parseFloat(getMapString(item, "5"))
	info := 0.0
	if val := getMapString(item, "7"); val != "" {
		info = parseFloat(val)
	} else if val := getMapString(item, "6"); val != "" {
		info = parseFloat(val)
	}
	return &banexg.Kline{
		Time:   stamp,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  closeP,
		Volume: vol,
		Info:   info,
	}
}

func parseWsMarkPriceItem(e *OKX, item map[string]interface{}) (string, float64, string, string) {
	if item == nil {
		return "", 0, "", ""
	}
	instId := getMapString(item, "instId")
	if instId == "" {
		return "", 0, "", ""
	}
	symbol := instId
	marketType := ""
	if e != nil {
		if market := getMarketByIDAny(e, instId, ""); market != nil {
			symbol = market.Symbol
			marketType = market.Type
		}
	}
	if marketType == "" {
		instType := getMapString(item, "instType")
		marketType = parseMarketType(instType, "")
	}
	price := parseFloat(getMapString(item, "markPx"))
	return symbol, price, marketType, instId
}

func updateAccLeverages(acc *banexg.Account, positions []*banexg.Position) []*banexg.AccountConfig {
	if acc == nil || len(positions) == 0 {
		return nil
	}
	acc.LockLeverage.Lock()
	defer acc.LockLeverage.Unlock()
	updates := make([]*banexg.AccountConfig, 0)
	for _, pos := range positions {
		if pos == nil || pos.Symbol == "" || pos.Leverage <= 0 {
			continue
		}
		cur, ok := acc.Leverages[pos.Symbol]
		if !ok || cur != pos.Leverage {
			acc.Leverages[pos.Symbol] = pos.Leverage
			updates = append(updates, &banexg.AccountConfig{Symbol: pos.Symbol, Leverage: pos.Leverage})
		}
	}
	return updates
}

func buildWsKey(channel, instId string) string {
	if instId == "" {
		return channel
	}
	return channel + ":" + instId
}

func buildWsKeyWithType(channel, instType, instId string) string {
	if instType != "" && instId != "" {
		return channel + ":" + instType + ":" + instId
	}
	if instType != "" {
		return channel + ":" + instType
	}
	return buildWsKey(channel, instId)
}

func parseWsKey(key string) (string, string, string) {
	parts := strings.Split(key, ":")
	if len(parts) == 1 {
		return parts[0], "", ""
	}
	if len(parts) == 2 {
		if isWsInstType(parts[1]) {
			return parts[0], parts[1], ""
		}
		return parts[0], "", parts[1]
	}
	return parts[0], parts[1], strings.Join(parts[2:], ":")
}

func isWsInstType(val string) bool {
	switch strings.ToUpper(val) {
	case "SPOT", "MARGIN", "SWAP", "FUTURES", "OPTION":
		return true
	default:
		return false
	}
}

func parseBoolString(val string) bool {
	switch strings.ToLower(val) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func getMapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	val, ok := m[key]
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprint(v)
	}
}

func getMapSlice(m map[string]interface{}, key string) []map[string]interface{} {
	val, ok := m[key]
	if !ok {
		return nil
	}
	raw, ok := val.([]interface{})
	if !ok {
		return nil
	}
	res := make([]map[string]interface{}, 0, len(raw))
	for _, it := range raw {
		if mp, ok := it.(map[string]interface{}); ok {
			res = append(res, mp)
		} else if arr, ok := it.([]interface{}); ok {
			m := make(map[string]interface{}, len(arr))
			for i, v := range arr {
				m[strconv.Itoa(i)] = v
			}
			res = append(res, m)
		}
	}
	return res
}

/*
makeCheckWsTimeout creates a goroutine that:
1. Periodically sends "ping" to all OKX WebSocket connections to keep them alive (OKX requires ping every <30s)
2. Checks for subscription timeout and resubscribes if needed
*/
func makeCheckWsTimeout(e *OKX) func() {
	pingData := []byte("ping")
	return func() {
		e.WsChecking = true
		defer func() {
			e.WsChecking = false
		}()
		// OKX requires ping every <30s, we use 20s interval
		pingInterval := time.Second * 20
		for {
			time.Sleep(pingInterval)
			for _, client := range e.WSClients {
				conns, lock := client.LockConns()
				for _, conn := range conns {
					// Send raw "ping" string to keep connection alive
					if err := client.WriteRaw(conn, pingData); err != nil {
						log.Warn("send ping fail", zap.String("url", client.URL),
							zap.Int("conn", conn.GetID()), zap.Error(err))
					}
				}
				lock.Unlock()
			}
		}
	}
}
