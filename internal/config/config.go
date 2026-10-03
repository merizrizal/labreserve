package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL  string
	ListenAddr   string
	PublicOrigin string
	CookieSecure bool
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:  strings.TrimSpace(os.Getenv("DATABASE_URL")),
		ListenAddr:   valueOr("LISTEN_ADDR", "127.0.0.1:8080"),
		PublicOrigin: valueOr("APP_ORIGIN", "http://127.0.0.1:8080"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	parsed, err := url.Parse(cfg.PublicOrigin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Config{}, fmt.Errorf("APP_ORIGIN must be an absolute origin without a path")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Config{}, fmt.Errorf("APP_ORIGIN must use http or https")
	}

	appEnv := strings.ToLower(valueOr("APP_ENV", "production"))
	cfg.CookieSecure = appEnv != "development" || parsed.Scheme == "https"
	if appEnv == "development" && parsed.Scheme == "http" && !isLoopbackListenAddr(cfg.ListenAddr) &&
		strings.ToLower(valueOr("APP_CONTAINERIZED", "false")) != "true" {
		return Config{}, fmt.Errorf("development HTTP must bind to loopback unless the container is published only on loopback")
	}
	return cfg, nil
}

func isLoopbackListenAddr(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func valueOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
