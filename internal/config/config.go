package config

import (
	"os"
	"time"
)

type Config struct {
	DefaultTimeout time.Duration
}

func Load() *Config {
	timeoutStr := os.Getenv("MTH_DEFAULT_TIMEOUT")
	if timeoutStr == "" {
		timeoutStr = "5m"
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 5 * time.Minute
	}

	return &Config{
		DefaultTimeout: timeout,
	}
}
