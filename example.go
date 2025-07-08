package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

type BTCUSDTSubscriber struct {
	symbol   string
	interval string
}

func (s *BTCUSDTSubscriber) SubscribeKLine(data KlineChannelMessageData) {
	kline := data.GetKlineData()
	fmt.Printf("[%s] %s - Open: %.4f, High: %.4f, Low: %.4f, Close: %.4f, Volume: %.2f\n",
		time.Unix(data.GetTimestamp()/1000, 0).Format("15:04:05"),
		data.GetSymbol(),
		kline.GetOpenPrice(),
		kline.GetHighPrice(),
		kline.GetLowPrice(),
		kline.GetClosePrice(),
		kline.GetBaseVolume(),
	)
}

func (s *BTCUSDTSubscriber) SubscribeInterval() string {
	return s.interval
}

func (s *BTCUSDTSubscriber) SubscribeSymbol() string {
	return s.symbol
}

func (s *BTCUSDTSubscriber) SubscribePriceType() string {
	return "close"
}

type ETHUSDTSubscriber struct {
	symbol   string
	interval string
}

func (s *ETHUSDTSubscriber) SubscribeKLine(data KlineChannelMessageData) {
	kline := data.GetKlineData()
	fmt.Printf("[%s] %s (%s) - Close: %.2f, Volume: %.2f\n",
		time.Unix(data.GetTimestamp()/1000, 0).Format("15:04:05"),
		data.GetSymbol(),
		s.interval,
		kline.GetClosePrice(),
		kline.GetBaseVolume(),
	)
}

func (s *ETHUSDTSubscriber) SubscribeInterval() string {
	return s.interval
}

func (s *ETHUSDTSubscriber) SubscribeSymbol() string {
	return s.symbol
}

func (s *ETHUSDTSubscriber) SubscribePriceType() string {
	return "close"
}

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer logger.Sync()

	client := NewBinanceWebSocketClient(logger)

	btcSubscriber := &BTCUSDTSubscriber{
		symbol:   "BTCUSDT",
		interval: "1m",
	}

	ethSubscriber1m := &ETHUSDTSubscriber{
		symbol:   "ETHUSDT",
		interval: "1m",
	}

	ethSubscriber5m := &ETHUSDTSubscriber{
		symbol:   "ETHUSDT",
		interval: "5m",
	}

	if err := client.SubscribeKLine(btcSubscriber); err != nil {
		logger.Fatal("Failed to subscribe to BTC kline", zap.Error(err))
	}

	if err := client.SubscribeKLine(ethSubscriber1m); err != nil {
		logger.Fatal("Failed to subscribe to ETH 1m kline", zap.Error(err))
	}

	if err := client.SubscribeKLine(ethSubscriber5m); err != nil {
		logger.Fatal("Failed to subscribe to ETH 5m kline", zap.Error(err))
	}

	logger.Info("Starting WebSocket client...")
	
	go func() {
		if err := client.Stream(); err != nil {
			logger.Fatal("WebSocket client failed", zap.Error(err))
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	fmt.Println("WebSocket client is running. Press Ctrl+C to stop.")
	fmt.Println("Subscribed to:")
	fmt.Println("- BTCUSDT 1m klines")
	fmt.Println("- ETHUSDT 1m klines") 
	fmt.Println("- ETHUSDT 5m klines")
	fmt.Println()

	time.Sleep(2 * time.Second)
	
	fmt.Println("Unsubscribing from ETHUSDT 5m after 10 seconds...")
	go func() {
		time.Sleep(10 * time.Second)
		if err := client.UnsubscribeKLine(ethSubscriber5m); err != nil {
			logger.Error("Failed to unsubscribe from ETH 5m", zap.Error(err))
		} else {
			fmt.Println("Successfully unsubscribed from ETHUSDT 5m")
		}
	}()

	<-c
	fmt.Println("\nShutting down...")
	client.Disconnect()
}