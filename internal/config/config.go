// Package config loads flixbus-mcp settings from FLIXBUS_* environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds runtime settings for the FlixBus API client and tools.
type Config struct {
	Currency string        // default currency for trip prices (ISO 4217)
	Locale   string        // locale for search results, underscore form (en_US)
	BaseURL  string        // API base URL, overridable for tests
	APIKey   string        // optional; sent as X-API-Key if set
	Timeout  time.Duration // HTTP client timeout
}

// FromEnv builds a Config from FLIXBUS_* env vars, applying defaults and validating.
func FromEnv() (Config, error) {
	cfg := Config{
		Currency: strings.ToUpper(getenv("FLIXBUS_CURRENCY", "EUR")),
		Locale:   getenv("FLIXBUS_LOCALE", "en_US"),
		BaseURL:  getenv("FLIXBUS_BASE_URL", "https://global.api.flixbus.com"),
		APIKey:   os.Getenv("FLIXBUS_API_KEY"),
	}
	if !ValidCurrency(cfg.Currency) {
		return Config{}, fmt.Errorf("FLIXBUS_CURRENCY %q: must be 3 ASCII letters", cfg.Currency)
	}
	timeout, err := time.ParseDuration(getenv("FLIXBUS_TIMEOUT", "30s"))
	if err != nil {
		return Config{}, fmt.Errorf("FLIXBUS_TIMEOUT: %w", err)
	}
	cfg.Timeout = timeout
	return cfg, nil
}

// ValidCurrency reports whether s is a plausible ISO 4217 code (3 ASCII letters).
func ValidCurrency(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
