package config_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/HuyDoan-2112/handle-massive-requests/internal/config"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     config.Config
	}{
		{
			name:     "explicit values",
			contents: "POSTGRES_URL=postgres://test/db\nGRPC_PORT=9091\n",
			want: config.Config{
				PostgresURL: "postgres://test/db",
				GRPCPort:    "9091",
			},
		},
		{
			name:     "default gRPC port",
			contents: "POSTGRES_URL=postgres://test/db\n",
			want: config.Config{
				PostgresURL: "postgres://test/db",
				GRPCPort:    "9090",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeEnvFile(t, tt.contents)

			got, err := config.LoadConfig()

			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			if got == nil || *got != tt.want {
				t.Errorf("LoadConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadConfigRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name:     "missing database URL",
			contents: "",
			wantErr:  "POSTGRES_URL is required",
		},
		{
			name:     "empty database URL",
			contents: "POSTGRES_URL=\n",
			wantErr:  "POSTGRES_URL is required",
		},
		{
			name:     "blank database URL",
			contents: "POSTGRES_URL=\"   \"\n",
			wantErr:  "POSTGRES_URL is required",
		},
		{
			name:     "malformed env file",
			contents: "POSTGRES_URL=\"unterminated\n",
			wantErr:  "failed to read config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeEnvFile(t, tt.contents)

			got, err := config.LoadConfig()

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("LoadConfig() error = %v, want error containing %q", err, tt.wantErr)
			}
			if got != nil {
				t.Errorf("LoadConfig() = %+v, want nil on error", got)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	t.Chdir(t.TempDir())

	got, err := config.LoadConfig()

	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("LoadConfig() error = %v, want a missing-file error", err)
	}
	if got != nil {
		t.Errorf("LoadConfig() = %+v, want nil on error", got)
	}
}

func writeEnvFile(t *testing.T, contents string) {
	t.Helper()
	if err := os.WriteFile(".env", []byte(contents), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}
