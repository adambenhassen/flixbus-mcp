package config_test

import (
	"testing"
	"time"

	"github.com/adambenhassen/flixbus-mcp/internal/config"
)

func TestFromEnvDefaults(t *testing.T) {
	// getenv treats empty as unset, so this pins defaults even when the
	// ambient environment exports FLIXBUS_* vars
	for _, k := range []string{
		"FLIXBUS_CURRENCY", "FLIXBUS_LOCALE", "FLIXBUS_BASE_URL",
		"FLIXBUS_API_KEY", "FLIXBUS_TIMEOUT",
	} {
		t.Setenv(k, "")
	}
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Currency != "EUR" {
		t.Errorf("Currency = %q, want EUR", cfg.Currency)
	}
	if cfg.Locale != "en_US" {
		t.Errorf("Locale = %q, want en_US", cfg.Locale)
	}
	if cfg.BaseURL != "https://global.api.flixbus.com" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("FLIXBUS_CURRENCY", "usd")
	t.Setenv("FLIXBUS_LOCALE", "de_DE")
	t.Setenv("FLIXBUS_TIMEOUT", "5s")
	t.Setenv("FLIXBUS_API_KEY", "k")
	t.Setenv("FLIXBUS_BASE_URL", "http://localhost:1")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Currency != "USD" {
		t.Errorf("Currency = %q, want USD (uppercased)", cfg.Currency)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
	if cfg.APIKey != "k" || cfg.BaseURL != "http://localhost:1" || cfg.Locale != "de_DE" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestFromEnvInvalid(t *testing.T) {
	t.Setenv("FLIXBUS_CURRENCY", "EU€")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error for invalid currency")
	}
	t.Setenv("FLIXBUS_CURRENCY", "EUR")
	t.Setenv("FLIXBUS_TIMEOUT", "banana")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error for invalid timeout")
	}
	// 0 would silently disable the HTTP timeout entirely
	t.Setenv("FLIXBUS_TIMEOUT", "0s")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error for zero timeout")
	}
	t.Setenv("FLIXBUS_TIMEOUT", "30s")
	t.Setenv("FLIXBUS_LOCALE", "english")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error for invalid locale")
	}
}

func TestFromEnvNormalizesHyphenLocale(t *testing.T) {
	t.Setenv("FLIXBUS_LOCALE", "en-US")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Locale != "en_US" {
		t.Errorf("Locale = %q, want en_US", cfg.Locale)
	}
}
