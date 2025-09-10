package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tradingiq/binance-client/interfaces"
	"github.com/tradingiq/binance-client/types"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

const (
	BinanceBaseWebSocketURL = "wss://fstream.binance.com"

	PingInterval = 3 * time.Minute

	PongTimeout = 10 * time.Minute

	DefaultMaxReconnectAttempts = 0
	DefaultReconnectDelay       = 15 * time.Second
)

type BaseClient struct {
	conn         *websocket.Conn
	clientCtx    context.Context
	clientCancel context.CancelFunc

	subscribers   map[string][]interfaces.KLineSubscriber
	activeStreams []string

	mu     sync.RWMutex
	logger *zap.Logger

	messageID uint
	idMu      sync.Mutex

	rateLimiter chan struct{}
	rateLimitMu sync.Mutex

	workerCtx    context.Context
	workerCancel context.CancelFunc
}

type ReconnectingClient struct {
	baseClient *BaseClient

	maxReconnectAttempts int
	reconnectDelay       time.Duration
	reconnectAttempts    int
	reconnectMu          sync.Mutex
}

type Client = ReconnectingClient

func NewBaseClient(logger *zap.Logger) *BaseClient {
	baseClient := &BaseClient{
		logger:        logger,
		rateLimiter:   make(chan struct{}, 8),
		subscribers:   make(map[string][]interfaces.KLineSubscriber),
		activeStreams: make([]string, 0),
	}

	return baseClient
}

func NewReconnectingClient(baseClient *BaseClient) *ReconnectingClient {
	return &ReconnectingClient{
		baseClient:           baseClient,
		maxReconnectAttempts: DefaultMaxReconnectAttempts,
		reconnectDelay:       DefaultReconnectDelay,
	}
}

func NewClient(logger *zap.Logger) *Client {
	return NewReconnectingClient(NewBaseClient(logger))
}

type ClientOption func(*ReconnectingClient)
type BaseClientOption func(*BaseClient)

func NewBaseClientWithOptions(logger *zap.Logger, opts ...BaseClientOption) *BaseClient {
	baseClient := NewBaseClient(logger)
	for _, opt := range opts {
		opt(baseClient)
	}
	return baseClient
}

func NewClientWithOptions(logger *zap.Logger, opts ...ClientOption) *Client {
	client := NewClient(logger)
	for _, opt := range opts {
		opt(client)
	}
	return client
}

func NewWebSocketClient(logger *zap.Logger) interfaces.PublicWebsocketClient {
	return NewClient(logger)
}

func NewWebSocketClientWithOptions(logger *zap.Logger, opts ...ClientOption) interfaces.PublicWebsocketClient {
	return NewClientWithOptions(logger, opts...)
}

func NewBaseWebSocketClient(logger *zap.Logger) interfaces.PublicWebsocketClient {
	return NewBaseClient(logger)
}

func NewBaseWebSocketClientWithOptions(logger *zap.Logger, opts ...BaseClientOption) interfaces.PublicWebsocketClient {
	return NewBaseClientWithOptions(logger, opts...)
}

func (c *BaseClient) refillRateLimiter() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.workerCtx.Done():
			return
		case <-ticker.C:

			select {
			case c.rateLimiter <- struct{}{}:
			default:

			}
		}
	}
}

func (c *BaseClient) acquireRateLimit() {
	select {
	case <-c.rateLimiter:
	case <-c.workerCtx.Done():
	}
}

func (c *BaseClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	if c.workerCancel != nil {
		c.workerCancel()
	}

	ctx, cancel := context.WithCancel(context.Background())

	url := BinanceBaseWebSocketURL
	if len(c.activeStreams) > 0 {
		url = fmt.Sprintf("%s/stream?streams=%s", BinanceBaseWebSocketURL, strings.Join(c.activeStreams, "/"))
	} else {

		url = fmt.Sprintf("%s/ws", BinanceBaseWebSocketURL)
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 5*time.Second)
	defer timeoutCancel()
	conn, _, err := websocket.Dial(timeoutCtx, url, nil)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}

	if len(c.rateLimiter) == 0 {
		for i := 0; i < 8; i++ {
			c.rateLimiter <- struct{}{}
		}
	}

	c.workerCtx, c.workerCancel = context.WithCancel(ctx)

	go c.refillRateLimiter()
	c.clientCtx = ctx
	c.clientCancel = cancel
	c.conn = conn
	c.logger.Info("Connected to Binance WebSocket", zap.String("url", url))

	return nil
}

func (c *BaseClient) Disconnect() {
	c.DisconnectWithCancel(true)
}

func (c *BaseClient) DisconnectWithCancel(cancelContext bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close(websocket.StatusNormalClosure, "client disconnect")

		c.conn = nil
	}

	if c.workerCancel != nil {
		c.workerCancel()
	}

	if cancelContext {
		c.clientCancel()
	}
	c.logger.Info("Disconnected from Binance WebSocket")
}

func (c *BaseClient) getNextMessageID() uint {
	c.idMu.Lock()
	defer c.idMu.Unlock()
	c.messageID++
	return c.messageID
}

func (c *BaseClient) sendSubscribeRequest(streams []string) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("websocket not connected")
	}

	return c.sendSubscribeRequestWithConn(conn, streams)
}

func (c *BaseClient) sendSubscribeRequestWithConn(conn *websocket.Conn, streams []string) error {
	c.acquireRateLimit()

	req := types.BinanceSubscribeRequest{
		Method: "SUBSCRIBE",
		Params: streams,
		ID:     c.getNextMessageID(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal subscribe request: %w", err)
	}

	if err := conn.Write(c.clientCtx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to send subscribe request: %w", err)
	}

	c.logger.Info("Sent subscribe request", zap.Strings("streams", streams), zap.Uint("id", req.ID))
	return nil
}

func (c *BaseClient) sendUnsubscribeRequest(streams []string) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("websocket not connected")
	}

	return c.sendUnsubscribeRequestWithConn(conn, streams)
}

func (c *BaseClient) sendUnsubscribeRequestWithConn(conn *websocket.Conn, streams []string) error {
	c.acquireRateLimit()

	req := types.BinanceSubscribeRequest{
		Method: "UNSUBSCRIBE",
		Params: streams,
		ID:     c.getNextMessageID(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal unsubscribe request: %w", err)
	}

	if err := conn.Write(c.clientCtx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to send unsubscribe request: %w", err)
	}

	c.logger.Info("Sent unsubscribe request", zap.Strings("streams", streams), zap.Uint("id", req.ID))
	return nil
}

// New Bitunix-compatible method
func (c *BaseClient) SubscribeKLine(subscription *interfaces.KLineSubscription) error {
	return c.SubscribeKLineWithSubscriber(subscription.Handler)
}

// Legacy method for backward compatibility
func (c *BaseClient) SubscribeKLineWithSubscriber(subscriber interfaces.KLineSubscriber) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return errors.New("websocket not connected")
	}

	symbol := strings.ToLower(subscriber.SubscribeSymbol())
	interval := subscriber.SubscribeInterval()
	streamName := fmt.Sprintf("%s@kline_%s", symbol, interval)

	isNewStream := false
	if _, exists := c.subscribers[streamName]; !exists {
		c.subscribers[streamName] = []interfaces.KLineSubscriber{}
		c.activeStreams = append(c.activeStreams, streamName)
		isNewStream = true
	}

	c.subscribers[streamName] = append(c.subscribers[streamName], subscriber)
	c.logger.Info("Subscribed to kline stream", zap.String("stream", streamName))

	if isNewStream && c.conn != nil {
		conn := c.conn
		go func() {
			if err := c.sendSubscribeRequestWithConn(conn, []string{streamName}); err != nil {
				c.logger.Error("Failed to send subscribe request", zap.String("stream", streamName), zap.Error(err))
			}
		}()
	}

	return nil
}

// New Bitunix-compatible method
func (c *BaseClient) UnsubscribeKLine(subscription *interfaces.KLineSubscription) error {
	return c.UnsubscribeKLineWithSubscriber(subscription.Handler)
}

// Legacy method for backward compatibility
func (c *BaseClient) UnsubscribeKLineWithSubscriber(subscriber interfaces.KLineSubscriber) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return errors.New("websocket not connected")
	}

	symbol := strings.ToLower(subscriber.SubscribeSymbol())
	interval := subscriber.SubscribeInterval()
	streamName := fmt.Sprintf("%s@kline_%s", symbol, interval)

	shouldUnsubscribe := false
	if subs, exists := c.subscribers[streamName]; exists {
		for i, sub := range subs {
			if sub == subscriber {
				c.subscribers[streamName] = append(subs[:i], subs[i+1:]...)
				break
			}
		}

		if len(c.subscribers[streamName]) == 0 {
			delete(c.subscribers, streamName)
			shouldUnsubscribe = true

			for i, stream := range c.activeStreams {
				if stream == streamName {
					c.activeStreams = append(c.activeStreams[:i], c.activeStreams[i+1:]...)
					break
				}
			}
		}
	}

	c.logger.Info("Unsubscribed from kline stream", zap.String("stream", streamName))

	if shouldUnsubscribe && c.conn != nil {
		conn := c.conn
		go func() {
			if err := c.sendUnsubscribeRequestWithConn(conn, []string{streamName}); err != nil {
				c.logger.Error("Failed to send unsubscribe request", zap.String("stream", streamName), zap.Error(err))
			}
		}()
	}

	return nil
}

func (c *BaseClient) Stream() error {
	if c.conn == nil {
		return fmt.Errorf("websocket not connected")
	}

	go c.handlePing()

	for c.conn != nil {
		select {
		case <-c.clientCtx.Done():
			return nil
		default:
			if err := c.readMessage(); err != nil {
				c.logger.Error("Failed to read message", zap.Error(err))
				c.DisconnectWithCancel(false)
				return err
			}
		}
	}
	return nil
}

func (c *BaseClient) readMessage() error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("websocket not connected")
	}

	_, message, err := conn.Read(c.clientCtx)
	if err != nil {
		return fmt.Errorf("failed to read message: %w", err)
	}

	var subResponse types.BinanceSubscribeResponse
	if err := json.Unmarshal(message, &subResponse); err == nil && subResponse.ID > 0 {
		c.logger.Info("Received subscription response", zap.Uint("id", subResponse.ID), zap.Any("result", subResponse.Result))
		return nil
	}

	var streamResponse types.BinanceStreamResponse
	if err := json.Unmarshal(message, &streamResponse); err == nil && streamResponse.Stream != "" {
		var klineEvent types.BinanceKlineEvent
		if err := json.Unmarshal(streamResponse.Data, &klineEvent); err != nil {
			c.logger.Debug("Failed to unmarshal kline event from combined stream", zap.Error(err))
			return nil
		}

		c.processKlineMessage(streamResponse.Stream, klineEvent)
		return nil
	}

	var klineEvent types.BinanceKlineEvent
	if err := json.Unmarshal(message, &klineEvent); err == nil && klineEvent.EventType == "kline" {
		streamName := fmt.Sprintf("%s@kline_%s", strings.ToLower(klineEvent.Symbol), klineEvent.KlineData.Interval)
		c.processKlineMessage(streamName, klineEvent)
		return nil
	}

	c.logger.Debug("Received unknown message type", zap.String("message", string(message)))
	return nil
}

func (c *BaseClient) processKlineMessage(streamName string, event types.BinanceKlineEvent) {
	c.mu.RLock()
	subscribers, exists := c.subscribers[streamName]
	c.mu.RUnlock()

	if !exists {
		return
	}

	channelMessage := types.NewKlineChannelMessage(
		streamName,
		event.EventTime,
		event.KlineData,
		event.Symbol,
	)

	for _, subscriber := range subscribers {
		go subscriber.SubscribeKLine(channelMessage)
	}
}

func (c *BaseClient) handlePing() {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.workerCtx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn != nil {
				ctx, cancel := context.WithTimeout(c.clientCtx, 5*time.Second)
				err := conn.Ping(ctx)
				cancel()
				if err != nil {
					c.logger.Error("Failed to send ping", zap.Error(err))
					c.DisconnectWithCancel(false)
				}
			}
		}
	}
}

// ReconnectingClient methods
func (c *ReconnectingClient) Connect() error {
	return c.baseClient.Connect()
}

func (c *ReconnectingClient) Disconnect() {
	c.baseClient.Disconnect()
}

// New Bitunix-compatible method
func (c *ReconnectingClient) SubscribeKLine(subscription *interfaces.KLineSubscription) error {
	return c.baseClient.SubscribeKLine(subscription)
}

// Legacy method for backward compatibility
func (c *ReconnectingClient) SubscribeKLineWithSubscriber(subscriber interfaces.KLineSubscriber) error {
	return c.baseClient.SubscribeKLineWithSubscriber(subscriber)
}

// New Bitunix-compatible method
func (c *ReconnectingClient) UnsubscribeKLine(subscription *interfaces.KLineSubscription) error {
	return c.baseClient.UnsubscribeKLine(subscription)
}

// Legacy method for backward compatibility
func (c *ReconnectingClient) UnsubscribeKLineWithSubscriber(subscriber interfaces.KLineSubscriber) error {
	return c.baseClient.UnsubscribeKLineWithSubscriber(subscriber)
}

func (c *ReconnectingClient) Stream() error {
	for {
		if c.baseClient.conn == nil {
			if err := c.connectWithReconnect(); err != nil {
				return err
			}
		}

		if err := c.baseClient.Stream(); err != nil {
			c.baseClient.logger.Error("Stream failed", zap.Error(err))
			c.baseClient.DisconnectWithCancel(false)
			continue
		}
	}
}

func (c *ReconnectingClient) connectWithReconnect() error {
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	for attempt := 1; attempt <= c.maxReconnectAttempts || c.maxReconnectAttempts == 0; attempt++ {
		c.baseClient.logger.Info("Attempting to connect", zap.Int("attempt", attempt), zap.Int("maxAttempts", c.maxReconnectAttempts))

		if err := c.baseClient.Connect(); err != nil {
			c.baseClient.logger.Error("Connection attempt failed", zap.Int("attempt", attempt), zap.Error(err))

			if attempt == c.maxReconnectAttempts {
				return fmt.Errorf("failed to connect after %d attempts: %w", c.maxReconnectAttempts, err)
			}

			c.baseClient.logger.Info("Waiting before next reconnect attempt", zap.Duration("delay", c.reconnectDelay))

			select {
			case <-time.After(c.reconnectDelay):
				continue
			case <-c.baseClient.clientCtx.Done():
				return c.baseClient.clientCtx.Err()
			}
		} else {
			c.reconnectAttempts = 0
			c.baseClient.logger.Info("Successfully connected", zap.Int("attempt", attempt))

			if err := c.resubscribeAll(); err != nil {
				c.baseClient.logger.Error("Failed to resubscribe to streams", zap.Error(err))
				c.baseClient.DisconnectWithCancel(false)
				continue
			}

			return nil
		}
	}

	return fmt.Errorf("exhausted all reconnection attempts")
}

func (c *ReconnectingClient) resubscribeAll() error {
	c.baseClient.mu.RLock()
	streams := make([]string, 0, len(c.baseClient.activeStreams))
	for _, stream := range c.baseClient.activeStreams {
		streams = append(streams, stream)
	}
	c.baseClient.mu.RUnlock()

	if len(streams) == 0 {
		return nil
	}

	c.baseClient.logger.Info("Resubscribing to streams", zap.Strings("streams", streams))
	return c.baseClient.sendSubscribeRequest(streams)
}
