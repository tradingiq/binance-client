package main

import (
	"fmt"
	"github.com/tradingiq/binance-client/interfaces"
	"github.com/tradingiq/binance-client/websocket"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

type KlineSubscriber struct {
	symbol    string
	interval  string
	priceType string
	logger    *zap.Logger
}

func (s *KlineSubscriber) SubscribeSymbol() string {
	return s.symbol
}

func (s *KlineSubscriber) SubscribeInterval() string {
	return s.interval
}

func (s *KlineSubscriber) SubscribePriceType() string {
	return s.priceType
}

func (s *KlineSubscriber) SubscribeKLine(message interfaces.KlineChannelMessageData) {
	kline := message.GetKlineData()
	s.logger.Info("Received kline data",
		zap.String("symbol", s.symbol),
		zap.String("interval", s.interval),
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

	btcSubscriber := &KlineSubscriber{
		symbol:    "BTCUSDT",
		interval:  "1m",
		priceType: "spot",
		logger:    logger,
	}

	ethSubscriber := &KlineSubscriber{
		symbol:    "ETHUSDT",
		interval:  "5m",
		priceType: "spot",
		logger:    logger,
	}

	linkSubscriber := &KlineSubscriber{
		symbol:    "LINKUSDT",
		interval:  "1m",
		priceType: "spot",
		logger:    logger,
	}

	if err := client.SubscribeKLine(btcSubscriber); err != nil {
		logger.Fatal("Failed to subscribe to BTC klines", zap.Error(err))
	}

	if err := client.SubscribeKLine(ethSubscriber); err != nil {
		logger.Fatal("Failed to subscribe to ETH klines", zap.Error(err))
	}

	go func() {
		if err := client.Stream(); err != nil {
			logger.Error("Stream error", zap.Error(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Streaming kline data for BTC and ETH. Press Ctrl+C to stop...")
	fmt.Println("LINK will be added after 15 seconds and removed after 45 seconds")

	go func() {
		time.Sleep(15 * time.Second)
		logger.Info("Adding LINKUSDT subscription...")
		if err := client.SubscribeKLine(linkSubscriber); err != nil {
			logger.Error("Failed to subscribe to LINK klines", zap.Error(err))
		} else {
			logger.Info("Successfully subscribed to LINKUSDT")
		}
	}()

	go func() {
		time.Sleep(30 * time.Second)
		logger.Info("Unsubscribing from ETH klines...")
		if err := client.UnsubscribeKLine(ethSubscriber); err != nil {
			logger.Error("Failed to unsubscribe from ETH", zap.Error(err))
		} else {
			logger.Info("Successfully unsubscribed from ETHUSDT")
		}
	}()

	go func() {
		time.Sleep(45 * time.Second)
		logger.Info("Unsubscribing from LINKUSDT klines...")
		if err := client.UnsubscribeKLine(linkSubscriber); err != nil {
			logger.Error("Failed to unsubscribe from LINK", zap.Error(err))
		} else {
			logger.Info("Successfully unsubscribed from LINKUSDT")
		}
	}()

	<-sigChan
	logger.Info("Shutting down...")

	client.UnsubscribeKLine(btcSubscriber)
	client.UnsubscribeKLine(linkSubscriber)
	client.Disconnect()

	fmt.Println("Goodbye!")
}
