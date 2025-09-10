package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/tradingiq/binance-client/interfaces"
	"github.com/tradingiq/binance-client/websocket"
	"go.uber.org/zap"
)

type MultiSymbolSubscriber struct {
	symbol   string
	interval string
	logger   *zap.Logger
	mu       sync.Mutex
	count    int
}

func (s *MultiSymbolSubscriber) SubscribeKLine(data interfaces.KlineChannelMessageData) {
	s.mu.Lock()
	s.count++
	s.mu.Unlock()

}

func (s *MultiSymbolSubscriber) SubscribePriceType() string {
	return "close"
}

func NewMultiSymbolSubscriber(symbol, interval string, logger *zap.Logger) *MultiSymbolSubscriber {
	return &MultiSymbolSubscriber{
		symbol:   symbol,
		interval: interval,
		logger:   logger,
	}
}

func (s *MultiSymbolSubscriber) SubscribeSymbol() string {
	return s.symbol
}

func (s *MultiSymbolSubscriber) SubscribeInterval() string {
	return s.interval
}

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	client := websocket.NewClient(logger)

	if err := client.Connect(); err != nil {
		log.Fatal("Failed to connect:", err)
	}

	symbols := []struct {
		symbol   string
		interval string
	}{
		{"BTCUSDT", "1m"},
		{"ETHUSDT", "1m"},
		{"BNBUSDT", "5m"},
		{"ADAUSDT", "15m"},
		{"XRPUSDT", "1h"},
		{"SOLUSDT", "1m"},
		{"DOTUSDT", "5m"},
		{"DOGEUSDT", "1m"},
		{"AVAXUSDT", "15m"},
		{"MATICUSDT", "1h"},
	}

	var subscribers []interfaces.KLineSubscriber
	subscribeStart := time.Now()

	logger.Info("Starting rapid subscription test with rate limiting")
	logger.Info("Rate limit: 8 requests per second with token bucket refill")

	for i, sym := range symbols {
		subscriber := NewMultiSymbolSubscriber(sym.symbol, sym.interval, logger)
		subscribers = append(subscribers, subscriber)

		logger.Info(fmt.Sprintf("Subscribing to symbol %d/%d", i+1, len(symbols)),
			zap.String("symbol", sym.symbol),
			zap.String("interval", sym.interval),
			zap.Time("timestamp", time.Now()),
		)

		err := client.SubscribeKLineWithSubscriber(subscriber)
		if err != nil {
			logger.Error("Failed to subscribe",
				zap.String("symbol", sym.symbol),
				zap.String("interval", sym.interval),
				zap.Error(err),
			)
		}
	}

	subscribeEnd := time.Now()
	logger.Info("All subscriptions queued",
		zap.Duration("totalTime", subscribeEnd.Sub(subscribeStart)),
		zap.Int("totalSymbols", len(symbols)),
	)

	go func() {
		if err := client.Stream(); err != nil {
			logger.Error("Stream error", zap.Error(err))
		}
	}()

	logger.Info("WebSocket client connected and streaming")
	logger.Info("Rate limiting ensures no more than 8 requests per second")
	logger.Info("Token bucket refills at 200ms intervals")

	logger.Info("Demonstrating burst subscription after 5 seconds...")
	time.Sleep(5 * time.Second)

	burstSymbols := []struct {
		symbol   string
		interval string
	}{
		{"LINKUSDT", "1m"},
		{"LTCUSDT", "1m"},
		{"UNIUSDT", "5m"},
		{"ATOMUSDT", "15m"},
		{"XLMUSDT", "1h"},
	}

	burstStart := time.Now()
	logger.Info("Starting burst subscription test")

	for i, sym := range burstSymbols {
		subscriber := NewMultiSymbolSubscriber(sym.symbol, sym.interval, logger)
		subscribers = append(subscribers, subscriber)

		logger.Info(fmt.Sprintf("Burst subscribing to symbol %d/%d", i+1, len(burstSymbols)),
			zap.String("symbol", sym.symbol),
			zap.String("interval", sym.interval),
			zap.Time("timestamp", time.Now()),
		)

		err := client.SubscribeKLineWithSubscriber(subscriber)
		if err != nil {
			logger.Error("Failed to subscribe",
				zap.String("symbol", sym.symbol),
				zap.String("interval", sym.interval),
				zap.Error(err),
			)
		}
	}

	burstEnd := time.Now()
	logger.Info("Burst subscriptions completed",
		zap.Duration("burstTime", burstEnd.Sub(burstStart)),
		zap.Int("burstSymbols", len(burstSymbols)),
	)

	logger.Info("Monitoring subscription updates for 30 seconds...")
	time.Sleep(30 * time.Second)

	logger.Info("Demonstrating unsubscribe with rate limiting")
	unsubStart := time.Now()

	for i := 0; i < 5 && i < len(subscribers); i++ {
		logger.Info(fmt.Sprintf("Unsubscribing from symbol %d/5", i+1),
			zap.String("symbol", subscribers[i].SubscribeSymbol()),
			zap.String("interval", subscribers[i].SubscribeInterval()),
			zap.Time("timestamp", time.Now()),
		)

		err := client.UnsubscribeKLineWithSubscriber(subscribers[i])
		if err != nil {
			logger.Error("Failed to unsubscribe",
				zap.String("symbol", subscribers[i].SubscribeSymbol()),
				zap.Error(err),
			)
		}
	}

	unsubEnd := time.Now()
	logger.Info("Unsubscribe operations completed",
		zap.Duration("unsubTime", unsubEnd.Sub(unsubStart)),
		zap.Int("unsubCount", 5),
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Press Ctrl+C to exit")
	<-sigChan

	logger.Info("Shutting down...")
	client.Disconnect()

	logger.Info("Summary:",
		zap.Int("totalSubscriptions", len(symbols)+len(burstSymbols)),
		zap.Duration("initialSubscriptionTime", subscribeEnd.Sub(subscribeStart)),
		zap.Duration("burstSubscriptionTime", burstEnd.Sub(burstStart)),
		zap.String("rateLimit", "8 requests/second with token bucket"),
	)
}
