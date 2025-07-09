package types

import (
	"strconv"
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
