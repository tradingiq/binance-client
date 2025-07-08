package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

const (
	BinanceWebSocketURL = "wss://fstream.binance.com/stream"
	PingInterval        = 3 * time.Minute
	PongTimeout         = 10 * time.Minute
	ReconnectDelay      = 5 * time.Second
	MaxReconnectRetries = 5
)

type BinanceKlineData struct {
	OpenTime                 int64  `json:"t"`
	CloseTime                int64  `json:"T"`
	Symbol                   string `json:"s"`
	Interval                 string `json:"i"`
	FirstTradeID             int64  `json:"f"`
	LastTradeID              int64  `json:"L"`
	OpenPrice                string `json:"o"`
	ClosePrice               string `json:"c"`
	HighPrice                string `json:"h"`
	LowPrice                 string `json:"l"`
	BaseAssetVolume          string `json:"v"`
	NumberOfTrades           int64  `json:"n"`
	IsKlineClosed            bool   `json:"x"`
	QuoteAssetVolume         string `json:"q"`
	TakerBuyBaseAssetVolume  string `json:"V"`
	TakerBuyQuoteAssetVolume string `json:"Q"`
	Ignore                   string `json:"B"`
}

func (k *BinanceKlineData) GetOpenPrice() float64 {
	price, _ := strconv.ParseFloat(k.OpenPrice, 64)
	return price
}

func (k *BinanceKlineData) GetClosePrice() float64 {
	price, _ := strconv.ParseFloat(k.ClosePrice, 64)
	return price
}

func (k *BinanceKlineData) GetHighPrice() float64 {
	price, _ := strconv.ParseFloat(k.HighPrice, 64)
	return price
}

func (k *BinanceKlineData) GetLowPrice() float64 {
	price, _ := strconv.ParseFloat(k.LowPrice, 64)
	return price
}

func (k *BinanceKlineData) GetBaseVolume() float64 {
	volume, _ := strconv.ParseFloat(k.BaseAssetVolume, 64)
	return volume
}

func (k *BinanceKlineData) GetQuoteVolume() float64 {
	volume, _ := strconv.ParseFloat(k.QuoteAssetVolume, 64)
	return volume
}

type BinanceKlineEvent struct {
	EventType string            `json:"e"`
	EventTime int64             `json:"E"`
	Symbol    string            `json:"s"`
	KlineData *BinanceKlineData `json:"k"`
}

type BinanceStreamResponse struct {
	Stream string            `json:"stream"`
	Data   BinanceKlineEvent `json:"data"`
}

type BinanceKlineChannelMessage struct {
	channel   string
	timestamp int64
	klineData KlineData
	symbol    string
}

func (b *BinanceKlineChannelMessage) GetChannel() string {
	return b.channel
}

func (b *BinanceKlineChannelMessage) GetTimestamp() int64 {
	return b.timestamp
}

func (b *BinanceKlineChannelMessage) GetKlineData() KlineData {
	return b.klineData
}

func (b *BinanceKlineChannelMessage) GetSymbol() string {
	return b.symbol
}

type BinanceWebSocketClient struct {
	conn        *websocket.Conn
	ctx         context.Context
	cancel      context.CancelFunc
	subscribers map[string][]KLineSubscriber
	mu          sync.RWMutex
	logger      *zap.Logger
	reconnect   chan struct{}
	isConnected bool
}

func NewBinanceWebSocketClient(logger *zap.Logger) *BinanceWebSocketClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &BinanceWebSocketClient{
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[string][]KLineSubscriber),
		logger:      logger,
		reconnect:   make(chan struct{}, 1),
	}
}

func (c *BinanceWebSocketClient) Connect() error {
	conn, _, err := websocket.Dial(c.ctx, BinanceWebSocketURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}

	c.conn = conn
	c.isConnected = true
	c.logger.Info("Connected to Binance WebSocket")

	return nil
}

func (c *BinanceWebSocketClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.isConnected = false
	if c.conn != nil {
		c.conn.Close(websocket.StatusNormalClosure, "client disconnect")
	}
	c.cancel()
	c.logger.Info("Disconnected from Binance WebSocket")
}

func (c *BinanceWebSocketClient) SubscribeKLine(subscriber KLineSubscriber) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	symbol := strings.ToLower(subscriber.SubscribeSymbol())
	interval := subscriber.SubscribeInterval()
	streamName := fmt.Sprintf("%s@kline_%s", symbol, interval)

	if _, exists := c.subscribers[streamName]; !exists {
		c.subscribers[streamName] = []KLineSubscriber{}
	}

	c.subscribers[streamName] = append(c.subscribers[streamName], subscriber)
	c.logger.Info("Subscribed to kline stream", zap.String("stream", streamName))

	return nil
}

func (c *BinanceWebSocketClient) UnsubscribeKLine(subscriber KLineSubscriber) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	symbol := strings.ToLower(subscriber.SubscribeSymbol())
	interval := subscriber.SubscribeInterval()
	streamName := fmt.Sprintf("%s@kline_%s", symbol, interval)

	if subs, exists := c.subscribers[streamName]; exists {
		for i, sub := range subs {
			if sub == subscriber {
				c.subscribers[streamName] = append(subs[:i], subs[i+1:]...)
				break
			}
		}

		if len(c.subscribers[streamName]) == 0 {
			delete(c.subscribers, streamName)
		}
	}

	c.logger.Info("Unsubscribed from kline stream", zap.String("stream", streamName))
	return nil
}

func (c *BinanceWebSocketClient) Stream() error {
	if !c.isConnected {
		if err := c.Connect(); err != nil {
			return err
		}
	}

	go c.handlePing()

	for {
		select {
		case <-c.ctx.Done():
			return nil
		case <-c.reconnect:
			if err := c.handleReconnect(); err != nil {
				c.logger.Error("Failed to reconnect", zap.Error(err))
				return err
			}
		default:
			if err := c.readMessage(); err != nil {
				c.logger.Error("Failed to read message", zap.Error(err))
				select {
				case c.reconnect <- struct{}{}:
				default:
				}
				continue
			}
		}
	}
}

func (c *BinanceWebSocketClient) readMessage() error {
	_, message, err := c.conn.Read(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to read message: %w", err)
	}

	var response BinanceStreamResponse
	if err := json.Unmarshal(message, &response); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	c.processKlineMessage(response)
	return nil
}

func (c *BinanceWebSocketClient) processKlineMessage(response BinanceStreamResponse) {
	c.mu.RLock()
	subscribers, exists := c.subscribers[response.Stream]
	c.mu.RUnlock()

	if !exists {
		return
	}

	channelMessage := &BinanceKlineChannelMessage{
		channel:   response.Stream,
		timestamp: response.Data.EventTime,
		klineData: response.Data.KlineData,
		symbol:    response.Data.Symbol,
	}

	for _, subscriber := range subscribers {
		go subscriber.SubscribeKLine(channelMessage)
	}
}

func (c *BinanceWebSocketClient) handlePing() {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.isConnected {
				ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
				err := c.conn.Ping(ctx)
				cancel()
				if err != nil {
					c.logger.Error("Failed to send ping", zap.Error(err))
					select {
					case c.reconnect <- struct{}{}:
					default:
					}
				}
			}
		}
	}
}

func (c *BinanceWebSocketClient) handleReconnect() error {
	c.logger.Info("Attempting to reconnect...")

	if c.conn != nil {
		c.conn.Close(websocket.StatusGoingAway, "reconnecting")
	}

	var err error
	for i := 0; i < MaxReconnectRetries; i++ {
		time.Sleep(ReconnectDelay)

		if err = c.Connect(); err == nil {
			c.logger.Info("Successfully reconnected", zap.Int("attempt", i+1))
			return nil
		}

		c.logger.Error("Reconnection attempt failed", zap.Int("attempt", i+1), zap.Error(err))
	}

	return fmt.Errorf("failed to reconnect after %d attempts: %w", MaxReconnectRetries, err)
}
