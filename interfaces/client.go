package interfaces

type PublicWebsocketClient interface {
	Stream() error

	Connect() error

	Disconnect()

	// Updated to match Bitunix pattern - takes subscription object instead of subscriber
	SubscribeKLine(subscription *KLineSubscription) error

	UnsubscribeKLine(subscription *KLineSubscription) error

	// Legacy methods for backward compatibility
	SubscribeKLineWithSubscriber(subscriber KLineSubscriber) error
	UnsubscribeKLineWithSubscriber(subscriber KLineSubscriber) error
}