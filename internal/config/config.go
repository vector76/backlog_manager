// Package config handles loading and validating the server configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds server configuration.
type Config struct {
	Port              int    `json:"port"`
	DataDir           string `json:"data_dir"`
	DashboardUser     string `json:"dashboard_user"`
	DashboardPassword string `json:"dashboard_password"`
	BeadsServerURL    string `json:"beads_server_url"`
}

// Load reads and validates a config JSON file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	if cfg.Port == 0 {
		return nil, fmt.Errorf("config: port must be set")
	}
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("config: data_dir must be set")
	}
	if cfg.DashboardUser == "" {
		return nil, fmt.Errorf("config: dashboard_user must be set")
	}
	if cfg.DashboardPassword == "" {
		return nil, fmt.Errorf("config: dashboard_password must be set")
	}
	return &cfg, nil
}
