package main

type PublicWebsocketClient interface {
	Stream() error
	Connect() error
	Disconnect()
	SubscribeKLine(subscriber KLineSubscriber) error
	UnsubscribeKLine(subscriber KLineSubscriber) error
}

type KLineSubscriber interface {
	SubscribeKLine(KlineChannelMessageData)
	SubscribeInterval() string
	SubscribeSymbol() string
	SubscribePriceType() string
}

type KlineData interface {
	GetOpenPrice() float64
	GetClosePrice() float64
	GetHighPrice() float64
	GetLowPrice() float64
	GetBaseVolume() float64
	GetQuoteVolume() float64
}
type KlineChannelMessageData interface {
	GetChannel() string
	GetTimestamp() int64
	GetKlineData() KlineData
	GetSymbol() string
}
