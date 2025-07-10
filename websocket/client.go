package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tradingiq/binance-client/interfaces"
	"github.com/tradingiq/binance-client/types"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

const (
	BinanceBaseWebSocketURL = "wss://fstream.binance.com"

	PingInterval = 3 * time.Minute

	PongTimeout = 10 * time.Minute
)

type subscribeRequest struct {
	streams []string
}

type unsubscribeRequest struct {
	streams   []string
	timestamp time.Time
}

type Client struct {
	conn          *websocket.Conn
	ctx           context.Context
	cancel        context.CancelFunc
	subscribers   map[string][]interfaces.KLineSubscriber
	mu            sync.RWMutex
	logger        *zap.Logger
	isConnected   bool
	activeStreams []string
	messageID     uint
	idMu          sync.Mutex

	rateLimiter        chan struct{}
	subscribeQueue     chan subscribeRequest
	unsubscribeQueue   chan unsubscribeRequest
	pendingUnsubscribe map[string]time.Time
	queueMu            sync.Mutex
}

func NewClient(logger *zap.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		ctx:                ctx,
		cancel:             cancel,
		subscribers:        make(map[string][]interfaces.KLineSubscriber),
		logger:             logger,
		activeStreams:      make([]string, 0),
		rateLimiter:        make(chan struct{}, 5),
		subscribeQueue:     make(chan subscribeRequest, 100),
		unsubscribeQueue:   make(chan unsubscribeRequest, 100),
		pendingUnsubscribe: make(map[string]time.Time),
	}

	for i := 0; i < 8; i++ {
		client.rateLimiter <- struct{}{}
	}

	go client.refillRateLimiter()
	go client.processQueues()

	return client
}

func NewWebSocketClient(logger *zap.Logger) interfaces.PublicWebsocketClient {
	return NewClient(logger)
}

func (c *Client) refillRateLimiter() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:

			select {
			case c.rateLimiter <- struct{}{}:
			default:

			}
		}
	}
}

func (c *Client) acquireRateLimit() {
	select {
	case <-c.rateLimiter:
	case <-c.ctx.Done():
	}
}

func (c *Client) processQueues() {
	unsubscribeDelay := 5 * time.Second
	checkInterval := 100 * time.Millisecond
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case req := <-c.subscribeQueue:
			// Cancel any pending unsubscribe for these streams
			c.queueMu.Lock()
			for _, stream := range req.streams {
				delete(c.pendingUnsubscribe, stream)
			}
			c.queueMu.Unlock()

			// Process subscribe immediately
			c.acquireRateLimit()
			if err := c.executeSendSubscribeRequest(req.streams); err != nil {
				c.logger.Error("Failed to send subscribe request", zap.Strings("streams", req.streams), zap.Error(err))
			}

		case req := <-c.unsubscribeQueue:
			// Add to pending with timestamp
			c.queueMu.Lock()
			for _, stream := range req.streams {
				c.pendingUnsubscribe[stream] = req.timestamp
			}
			c.queueMu.Unlock()

		case <-ticker.C:
			// Check for pending unsubscribes that have waited long enough
			c.processPendingUnsubscribes(unsubscribeDelay)
		}
	}
}

func (c *Client) processPendingUnsubscribes(delay time.Duration) {
	c.queueMu.Lock()
	now := time.Now()
	streamsToUnsubscribe := []string{}
	
	for stream, timestamp := range c.pendingUnsubscribe {
		if now.Sub(timestamp) >= delay {
			streamsToUnsubscribe = append(streamsToUnsubscribe, stream)
			delete(c.pendingUnsubscribe, stream)
		}
	}
	c.queueMu.Unlock()

	if len(streamsToUnsubscribe) > 0 {
		// Check if there are any subscribe requests waiting
		select {
		case req := <-c.subscribeQueue:
			// Process subscribe first, then re-queue the unsubscribe
			c.queueMu.Lock()
			for _, stream := range req.streams {
				delete(c.pendingUnsubscribe, stream)
			}
			c.queueMu.Unlock()

			c.acquireRateLimit()
			if err := c.executeSendSubscribeRequest(req.streams); err != nil {
				c.logger.Error("Failed to send subscribe request", zap.Strings("streams", req.streams), zap.Error(err))
			}
			
			// Re-add unsubscribes that weren't cancelled
			c.queueMu.Lock()
			for _, stream := range streamsToUnsubscribe {
				if _, cancelled := c.pendingUnsubscribe[stream]; !cancelled {
					// Check if stream is not in the just-subscribed list
					isSubscribed := false
					for _, subStream := range req.streams {
						if subStream == stream {
							isSubscribed = true
							break
						}
					}
					if !isSubscribed {
						c.pendingUnsubscribe[stream] = now
					}
				}
			}
			c.queueMu.Unlock()
		default:
			// No subscribes waiting, process unsubscribe
			c.acquireRateLimit()
			if err := c.executeSendUnsubscribeRequest(streamsToUnsubscribe); err != nil {
				c.logger.Error("Failed to send unsubscribe request", zap.Strings("streams", streamsToUnsubscribe), zap.Error(err))
			}
		}
	}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected {
		return nil
	}

	url := BinanceBaseWebSocketURL
	if len(c.activeStreams) > 0 {
		url = fmt.Sprintf("%s/stream?streams=%s", BinanceBaseWebSocketURL, strings.Join(c.activeStreams, "/"))
	} else {

		url = fmt.Sprintf("%s/ws", BinanceBaseWebSocketURL)
	}

	conn, _, err := websocket.Dial(c.ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}

	c.conn = conn
	c.isConnected = true
	c.logger.Info("Connected to Binance WebSocket", zap.String("url", url))

	return nil
}

func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.isConnected = false
	if c.conn != nil {
		c.conn.Close(websocket.StatusNormalClosure, "client disconnect")
	}
	c.cancel()
	c.logger.Info("Disconnected from Binance WebSocket")
}

func (c *Client) getNextMessageID() uint {
	c.idMu.Lock()
	defer c.idMu.Unlock()
	c.messageID++
	return c.messageID
}

func (c *Client) sendSubscribeRequest(streams []string) error {
	select {
	case c.subscribeQueue <- subscribeRequest{streams: streams}:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("context cancelled")
	}
}

func (c *Client) executeSendSubscribeRequest(streams []string) error {
	if !c.isConnected || c.conn == nil {
		return fmt.Errorf("websocket not connected")
	}

	req := types.BinanceSubscribeRequest{
		Method: "SUBSCRIBE",
		Params: streams,
		ID:     c.getNextMessageID(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal subscribe request: %w", err)
	}

	if err := c.conn.Write(c.ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to send subscribe request: %w", err)
	}

	c.logger.Info("Sent subscribe request", zap.Strings("streams", streams), zap.Uint("id", req.ID))
	return nil
}

func (c *Client) sendUnsubscribeRequest(streams []string) error {
	select {
	case c.unsubscribeQueue <- unsubscribeRequest{streams: streams, timestamp: time.Now()}:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("context cancelled")
	}
}

func (c *Client) executeSendUnsubscribeRequest(streams []string) error {
	if !c.isConnected || c.conn == nil {
		return fmt.Errorf("websocket not connected")
	}

	req := types.BinanceSubscribeRequest{
		Method: "UNSUBSCRIBE",
		Params: streams,
		ID:     c.getNextMessageID(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal unsubscribe request: %w", err)
	}

	if err := c.conn.Write(c.ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to send unsubscribe request: %w", err)
	}

	c.logger.Info("Sent unsubscribe request", zap.Strings("streams", streams), zap.Uint("id", req.ID))
	return nil
}

func (c *Client) SubscribeKLine(subscriber interfaces.KLineSubscriber) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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

	if isNewStream && c.isConnected {
		go func() {
			if err := c.sendSubscribeRequest([]string{streamName}); err != nil {
				c.logger.Error("Failed to send subscribe request", zap.String("stream", streamName), zap.Error(err))
			}
		}()
	}

	return nil
}

func (c *Client) UnsubscribeKLine(subscriber interfaces.KLineSubscriber) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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

	if shouldUnsubscribe && c.isConnected {
		go func() {
			if err := c.sendUnsubscribeRequest([]string{streamName}); err != nil {
				c.logger.Error("Failed to send unsubscribe request", zap.String("stream", streamName), zap.Error(err))
			}
		}()
	}

	return nil
}

func (c *Client) Stream() error {
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
		default:
			if err := c.readMessage(); err != nil {
				c.logger.Error("Failed to read message", zap.Error(err))

				return err
			}
		}
	}
}

func (c *Client) readMessage() error {
	_, message, err := c.conn.Read(c.ctx)
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

func (c *Client) processKlineMessage(streamName string, event types.BinanceKlineEvent) {
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

func (c *Client) handlePing() {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.isConnected && c.conn != nil {
				ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
				err := c.conn.Ping(ctx)
				cancel()
				if err != nil {
					c.logger.Error("Failed to send ping", zap.Error(err))
				}
			}
		}
	}
}
