package main

import (
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

type RateLimiter struct {
	Limit int `yaml:"limit"`
	Burst int `yaml:"burst"`
}

type Cache struct {
	StandardTTL time.Duration `yaml:"standard_ttl"`
	Keys        time.Duration `yaml:"keys"`
}

type Config struct {
	Prod           bool                   `yaml:"prod"`
	Domain         string                 `yaml:"domain"`
	URL            string                 `yaml:"url"`
	Port           int                    `yaml:"port"`
	HTTPS          bool                   `yaml:"https"`
	ForwardHTTP    bool                   `yaml:"forward_http"`
	KeyPath        string                 `yaml:"key_path"`
	CertPath       string                 `yaml:"cert_path"`
	Secret         string                 `yaml:"secret"`
	AdminEmails    []string               `yaml:"admin_emails"`
	PermittedIPs   []string               `yaml:"permitted_ips"`
	TrustedProxies []string               `yaml:"trusted_proxies"`
	Cache          Cache                  `yaml:"cache"`
	RateLimiters   map[string]RateLimiter `yaml:"rate_limiters"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if config.Domain == "" {
		config.Domain = "localhost"
	}

	if config.URL == "" {
		config.URL = fmt.Sprintf("https://%s", config.Domain)
	}

	if config.Port == 0 {
		config.Port = 8080
	}

	if config.HTTPS {
		if config.KeyPath == "" {
			return nil, fmt.Errorf("key_path is required when https is enabled")
		}
		if config.CertPath == "" {
			return nil, fmt.Errorf("cert_path is required when https is enabled")
		}

		if _, err := os.Stat(config.KeyPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("key file does not exist: %s", config.KeyPath)
		}
		if _, err := os.Stat(config.CertPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("cert file does not exist: %s", config.CertPath)
		}

		if config.ForwardHTTP && config.Port == 443 {
			return nil, fmt.Errorf("cannot forward HTTP when using port 443 for HTTPS")
		}
	} else {
		if config.ForwardHTTP {
			return nil, fmt.Errorf("forward_http cannot be enabled when https is disabled")
		}
	}

	if config.Secret == "" {
		return nil, fmt.Errorf("secret is required")
	}

	if len(config.AdminEmails) == 0 {
		return nil, fmt.Errorf("at least one admin email is required")
	}

	if len(config.PermittedIPs) == 0 {
		config.PermittedIPs = []string{"*"}
	}

	if len(config.TrustedProxies) == 0 {
		config.TrustedProxies = []string{"127.0.0.1", "::1"}
	}

	if config.Cache.StandardTTL == 0 {
		config.Cache.StandardTTL = 60 * time.Minute
	}
	if config.Cache.Keys == 0 {
		config.Cache.Keys = 1 * time.Minute
	}

	if config.RateLimiters == nil {
		config.RateLimiters = make(map[string]RateLimiter)
	}
	if _, exists := config.RateLimiters["default"]; !exists {
		config.RateLimiters["default"] = RateLimiter{Limit: 5, Burst: 10}
	}

	if _, exists := config.RateLimiters["authorized_users"]; !exists {
		config.RateLimiters["authorized_users"] = RateLimiter{Limit: 100, Burst: 150}
	}

	if config.Prod {
		color.HiMagenta("running in production mode")
	} else {
		color.HiCyan("running in development mode")
	}

	return &config, nil
}
