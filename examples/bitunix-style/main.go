package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tradingiq/binance-client/interfaces"
	"github.com/tradingiq/binance-client/websocket"

	"go.uber.org/zap"
)

// KlineHandler implements the KLineSubscriber interface
type KlineHandler struct {
	symbol    string
	interval  string
	priceType string
	logger    *zap.Logger
}

func (h *KlineHandler) SubscribeSymbol() string {
	return h.symbol
}

func (h *KlineHandler) SubscribeInterval() string {
	return h.interval
}

func (h *KlineHandler) SubscribePriceType() string {
	return h.priceType
}

func (h *KlineHandler) SubscribeKLine(message interfaces.KlineChannelMessageData) {
	kline := message.GetKlineData()
	h.logger.Info("Received kline data",
		zap.String("symbol", h.symbol),
		zap.String("interval", h.interval),
		zap.String("channel", message.GetChannel()),
		zap.Int64("timestamp", message.GetTimestamp()),
		zap.Float64("open", kline.GetOpenPrice()),
		zap.Float64("high", kline.GetHighPrice()),
		zap.Float64("low", kline.GetLowPrice()),
		zap.Float64("close", kline.GetClosePrice()),
		zap.Float64("volume", kline.GetBaseVolume()),
	)
}

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	client := websocket.NewWebSocketClient(logger)

	// New Bitunix-style subscription pattern
	btcHandler := &KlineHandler{
		symbol:    "BTCUSDT",
		interval:  "1m",
		priceType: "spot",
		logger:    logger,
	}

	ethHandler := &KlineHandler{
		symbol:    "ETHUSDT",
		interval:  "5m",
		priceType: "spot",
		logger:    logger,
	}

	// Create subscriptions using the new pattern (similar to Bitunix)
	btcSubscription := interfaces.NewKLineSubscription("BTCUSDT", "1m", "spot", btcHandler)
	ethSubscription := interfaces.NewKLineSubscription("ETHUSDT", "5m", "spot", ethHandler)

	if err := client.Connect(); err != nil {
		logger.Fatal("Failed to connect", zap.Error(err))
	}

	// Subscribe using new Bitunix-compatible method
	if err := client.SubscribeKLine(btcSubscription); err != nil {
		logger.Fatal("Failed to subscribe to BTC klines", zap.Error(err))
	}

	if err := client.SubscribeKLine(ethSubscription); err != nil {
		logger.Fatal("Failed to subscribe to ETH klines", zap.Error(err))
	}

	go func() {
		if err := client.Stream(); err != nil {
			logger.Error("Stream error", zap.Error(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Streaming kline data using Bitunix-style interface. Press Ctrl+C to stop...")
	fmt.Println("This example shows the new subscription pattern that matches Bitunix client interface")

	go func() {
		time.Sleep(30 * time.Second)
		logger.Info("Unsubscribing from ETH klines using new method...")
		if err := client.UnsubscribeKLine(ethSubscription); err != nil {
			logger.Error("Failed to unsubscribe from ETH", zap.Error(err))
		} else {
			logger.Info("Successfully unsubscribed from ETHUSDT")
		}
	}()

	<-sigChan
	logger.Info("Shutting down...")

	client.UnsubscribeKLine(btcSubscription)
	client.Disconnect()

	fmt.Println("Goodbye!")
}