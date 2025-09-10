# Bitunix Compatibility Interface

This project now supports a Bitunix-compatible interface for KLine subscriptions, allowing easy switching between Binance and Bitunix clients.

## New Bitunix-Style Interface

```go
package main

import (
    "github.com/tradingiq/binance-client/interfaces"
    "github.com/tradingiq/binance-client/websocket"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()
    client := websocket.NewWebSocketClient(logger)

    // Create a handler that implements KLineSubscriber
    handler := &KlineHandler{
        symbol:    "BTCUSDT",
        interval:  "1m", 
        priceType: "spot",
        logger:    logger,
    }

    // Create subscription using new pattern (similar to Bitunix)
    subscription := interfaces.NewKLineSubscription("BTCUSDT", "1m", "spot", handler)

    // Connect and subscribe using new Bitunix-compatible method
    client.Connect()
    client.SubscribeKLine(subscription)  // New method
    client.Stream()
}
```

## Legacy Interface (Backward Compatible)

```go
// Legacy method still available
client.SubscribeKLineWithSubscriber(handler)
client.UnsubscribeKLineWithSubscriber(handler)
```

## Key Differences from Bitunix

**Similarity with Bitunix:**
- Uses subscription objects instead of passing subscriber directly
- Clean separation between subscription configuration and handler logic
- Easy to switch between clients by changing imports

**Bitunix pattern:**
```go
ws := bitunix.NewPublicWebsocket(ctx)
subscription := &KLineSubscriber{
    symbol: symbol,
    interval: interval,
    priceType: priceType,
}
ws.SubscribeKLine(subscription)
```

**Your updated Binance client:**
```go
ws := websocket.NewWebSocketClient(logger)
subscription := interfaces.NewKLineSubscription(symbol, interval, priceType, handler)
ws.SubscribeKLine(subscription)
```

## Migration Guide

1. **For new code**: Use the new `SubscribeKLine(subscription)` method
2. **For existing code**: Continue using `SubscribeKLineWithSubscriber(subscriber)` - no changes needed
3. **To match Bitunix exactly**: Replace your websocket client constructor and subscription creation only

## Examples

- `examples/bitunix-style/` - Shows new Bitunix-compatible interface
- `examples/kline/` - Shows legacy interface (still works)
- `examples/ratelimit/` - Updated to use legacy methods for compatibility

Both interfaces provide the same functionality with full reconnection, rate limiting, and subscription management capabilities.