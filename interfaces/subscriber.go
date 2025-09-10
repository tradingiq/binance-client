package interfaces

// KLineSubscriber represents a handler for kline data
type KLineSubscriber interface {
	SubscribeKLine(KlineChannelMessageData)

	SubscribeInterval() string

	SubscribeSymbol() string

	SubscribePriceType() string
}

// KLineSubscription represents a kline subscription configuration
// This matches the Bitunix pattern for easy switching between clients
type KLineSubscription struct {
	Symbol    string
	Interval  string
	PriceType string
	Handler   KLineSubscriber
}

// NewKLineSubscription creates a new kline subscription
func NewKLineSubscription(symbol, interval, priceType string, handler KLineSubscriber) *KLineSubscription {
	return &KLineSubscription{
		Symbol:    symbol,
		Interval:  interval,
		PriceType: priceType,
		Handler:   handler,
	}
}
