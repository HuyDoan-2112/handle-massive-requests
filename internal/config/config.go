package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	PostgresURL string `mapstructure:"POSTGRES_URL"`
	GRPCPort    string `mapstructure:"GRPC_PORT"`
}

func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetDefault("GRPC_PORT", "9090")
	err := v.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{
		PostgresURL: v.GetString("POSTGRES_URL"),
		GRPCPort:    v.GetString("GRPC_PORT"),
	}
	if strings.TrimSpace(cfg.PostgresURL) == "" {
		return nil, errors.New("POSTGRES_URL is required")
	}
	return cfg, nil
}
