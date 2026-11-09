package config

import (
	"fmt"
	"github.com/spf13/viper"
)

type Config struct {
	PostgresURL string `mapstructure:"POSTGRES_URL"`
	GinMode     string `mapstructure:"GIN_MODE"`
	Port        string `mapstructure:"PORT"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigFile(".env")
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return &Config{
		PostgresURL: viper.GetString("POSTGRES_URL"),
		GinMode:     viper.GetString("GIN_MODE"),
		Port:        viper.GetString("PORT"),
	}, nil
}
