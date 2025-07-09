package interfaces

type KLineSubscriber interface {
	SubscribeKLine(KlineChannelMessageData)

	SubscribeInterval() string

	SubscribeSymbol() string

	SubscribePriceType() string
}
