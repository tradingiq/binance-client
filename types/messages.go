package types

import (
	"encoding/json"
	"github.com/tradingiq/binance-client/interfaces"
)

type BinanceKlineEvent struct {
	EventType string            `json:"e"`
	EventTime int64             `json:"E"`
	Symbol    string            `json:"s"`
	KlineData *BinanceKlineData `json:"k"`
}

type BinanceStreamResponse struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

type BinanceSubscribeRequest struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     uint     `json:"id"`
}

type BinanceSubscribeResponse struct {
	Result interface{} `json:"result"`
	ID     uint        `json:"id"`
}

type BinanceKlineChannelMessage struct {
	channel   string
	timestamp int64
	klineData interfaces.KlineData
	symbol    string
}

func (b *BinanceKlineChannelMessage) GetChannel() string {
	return b.channel
}

func (b *BinanceKlineChannelMessage) GetTimestamp() int64 {
	return b.timestamp
}

func (b *BinanceKlineChannelMessage) GetKlineData() interfaces.KlineData {
	return b.klineData
}

func (b *BinanceKlineChannelMessage) GetSymbol() string {
	return b.symbol
}

func NewKlineChannelMessage(channel string, timestamp int64, klineData interfaces.KlineData, symbol string) interfaces.KlineChannelMessageData {
	return &BinanceKlineChannelMessage{
		channel:   channel,
		timestamp: timestamp,
		klineData: klineData,
		symbol:    symbol,
	}
}
