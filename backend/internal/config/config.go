// Package config loads and validates server configuration before any listener starts.
package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL           string
	AppOrigin             string
	AdminEmail            string
	AdminPassword         string
	Address               string
	SecureCookies         bool
	AllowLoopbackWebhooks bool
}

func Load() (Config, error) {
	settings := Config{DatabaseURL: os.Getenv("DATABASE_URL"), AppOrigin: os.Getenv("APP_ORIGIN"), AdminEmail: strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))), AdminPassword: os.Getenv("ADMIN_PASSWORD"), Address: ":8080"}
	if settings.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if err := ValidateOrigin(settings.AppOrigin); err != nil {
		return Config{}, err
	}
	settings.SecureCookies = strings.HasPrefix(settings.AppOrigin, "https://")
	settings.AllowLoopbackWebhooks = loopbackWebhooksAllowed(settings.AppOrigin)
	return settings, nil
}

// Loopback webhook URLs are accepted only for the local HTTP development origin.
func loopbackWebhooksAllowed(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1")
}
func ValidateOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil {
		return errors.New("APP_ORIGIN is invalid")
	}
	if parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return errors.New("APP_ORIGIN must be an origin without a path")
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || (parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1")) {
		return errors.New("APP_ORIGIN requires HTTPS except on localhost")
	}
	return nil
}
