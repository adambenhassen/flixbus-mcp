// Package config loads flixbus-mcp settings from FLIXBUS_* environment variables.
package config

import (
	"cmp"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// localeRe matches the underscore locale form the search API expects: a
// two-letter language with an optional territory, e.g. en, en_US, de_DE.
var localeRe = regexp.MustCompile(`^[A-Za-z]{2}(_[A-Za-z]{2,3})?$`)

// Config holds runtime settings for the FlixBus API client and tools.
type Config struct {
	Currency string        // default currency for trip prices (ISO 4217)
	Locale   string        // locale for search results, underscore form (en_US)
	BaseURL  string        // API base URL, overridable for tests
	APIKey   string        // optional; sent as X-Api-Key if set
	Timeout  time.Duration // HTTP client timeout
}

// FromEnv builds a Config from FLIXBUS_* env vars, applying defaults and validating.
func FromEnv() (Config, error) {
	cfg := Config{
		Currency: strings.ToUpper(cmp.Or(os.Getenv("FLIXBUS_CURRENCY"), "EUR")),
		// hyphen locales (en-US) are normalized to the API's underscore form
		Locale:  strings.ReplaceAll(cmp.Or(os.Getenv("FLIXBUS_LOCALE"), "en_US"), "-", "_"),
		BaseURL: cmp.Or(os.Getenv("FLIXBUS_BASE_URL"), "https://global.api.flixbus.com"),
		APIKey:  os.Getenv("FLIXBUS_API_KEY"),
	}
	if !ValidCurrency(cfg.Currency) {
		return Config{}, fmt.Errorf("FLIXBUS_CURRENCY %q: must be 3 uppercase ASCII letters", cfg.Currency)
	}
	if !localeRe.MatchString(cfg.Locale) {
		return Config{}, fmt.Errorf("FLIXBUS_LOCALE %q: want a locale like en_US", cfg.Locale)
	}
	timeout, err := time.ParseDuration(cmp.Or(os.Getenv("FLIXBUS_TIMEOUT"), "30s"))
	if err != nil {
		return Config{}, fmt.Errorf("FLIXBUS_TIMEOUT: %w", err)
	}
	if timeout <= 0 {
		return Config{}, fmt.Errorf("FLIXBUS_TIMEOUT %v: must be positive (0 would disable the HTTP timeout)", timeout)
	}
	cfg.Timeout = timeout
	return cfg, nil
}

// ValidCurrency reports whether s is a plausible ISO 4217 code (3 uppercase
// ASCII letters). Callers uppercase user input before validating.
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
