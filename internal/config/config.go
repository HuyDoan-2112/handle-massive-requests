package config

import (
	"fmt"
	"github.com/spf13/viper"
)

type Config struct {
	PostgresURL string `mapstructure:"POSTGRES_URL"`
	GRPCPort    string `mapstructure:"GRPC_PORT"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetDefault("GRPC_PORT", "9090")
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return &Config{
		PostgresURL: viper.GetString("POSTGRES_URL"),
		GRPCPort:    viper.GetString("GRPC_PORT"),
	}, nil
}
