package binance

import "github.com/banbox/banexg/base"

const (
	HostDApiPublic    = "dapiPublic"
	HostDApiPrivate   = "dapiPrivate"
	HostDApiPrivateV2 = "dapiPrivateV2"
	HostFApiPublic    = "fapiPublic"
	HostFApiPublicV2  = "fapiPublicV2"
	HostFApiPrivate   = "fapiPrivate"
	HostFApiPrivateV2 = "fapiPrivateV2"
	HostPublic        = "public"
	HostPrivate       = "private"
	HostV1            = "v1"
	HostSApi          = "sapi"
	HostSApiV2        = "sapiV2"
	HostSApiV3        = "sapiV3"
	HostSApiV4        = "sapiV4"
	HostEApiPublic    = "eapiPublic"
	HostEApiPrivate   = "eapiPrivate"
	HostDApiData      = "dapiData"
	HostFApiData      = "fapiData"
	HostPApi          = "papi"
	WssApi            = "ws"
)

const (
	OptRecvWindow = "RecvWindow"
)

var (
	DefCareMarkets = []string{
		base.MarketSpot, base.MarketLinear, base.MarketInverse,
	}
)

const (
	OdStatusNew             = "NEW"
	OdStatusPartiallyFilled = "PARTIALLY_FILLED"
	OdStatusAccept          = "ACCEPTED"
	OdStatusFilled          = "FILLED"
	OdStatusCanceled        = "CANCELED"
	OdStatusCancelled       = "CANCELLED"
	OdStatusPendingCancel   = "PENDING_CANCEL"
	OdStatusReject          = "REJECTED"
	OdStatusExpired         = "EXPIRED"
	OdStatusExpiredInMatch  = "EXPIRED_IN_MATCH"
)
