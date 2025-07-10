package types

import (
	"testing"
)

func TestBinanceKlineData_GetPrices(t *testing.T) {
	kline := &BinanceKlineData{
		OpenPrice:        "100.50",
		ClosePrice:       "101.75",
		HighPrice:        "102.00",
		LowPrice:         "99.25",
		BaseAssetVolume:  "1000.50",
		QuoteAssetVolume: "101250.75",
	}

	tests := []struct {
		name     string
		method   func() float64
		expected float64
	}{
		{"GetOpenPrice", kline.GetOpenPrice, 100.50},
		{"GetClosePrice", kline.GetClosePrice, 101.75},
		{"GetHighPrice", kline.GetHighPrice, 102.00},
		{"GetLowPrice", kline.GetLowPrice, 99.25},
		{"GetBaseVolume", kline.GetBaseVolume, 1000.50},
		{"GetQuoteVolume", kline.GetQuoteVolume, 101250.75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method()
			if result != tt.expected {
				t.Errorf("%s = %f, expected %f", tt.name, result, tt.expected)
			}
		})
	}
}

func TestBinanceKlineData_InvalidPrices(t *testing.T) {
	kline := &BinanceKlineData{
		OpenPrice:        "invalid",
		ClosePrice:       "",
		HighPrice:        "not-a-number",
		LowPrice:         "99.25",
		BaseAssetVolume:  "1000.50",
		QuoteAssetVolume: "101250.75",
	}

	if kline.GetOpenPrice() != 0 {
		t.Errorf("GetOpenPrice() with invalid price should return 0, got %f", kline.GetOpenPrice())
	}

	if kline.GetClosePrice() != 0 {
		t.Errorf("GetClosePrice() with empty price should return 0, got %f", kline.GetClosePrice())
	}

	if kline.GetHighPrice() != 0 {
		t.Errorf("GetHighPrice() with invalid price should return 0, got %f", kline.GetHighPrice())
	}

	if kline.GetLowPrice() != 99.25 {
		t.Errorf("GetLowPrice() = %f, expected 99.25", kline.GetLowPrice())
	}
}
