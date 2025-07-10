package interfaces

type PublicWebsocketClient interface {
	Stream() error

	Connect() error

	Disconnect()

	SubscribeKLine(subscriber KLineSubscriber) error

	UnsubscribeKLine(subscriber KLineSubscriber) error
}
