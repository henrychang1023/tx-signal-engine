// Package shioajiproc manages the Shioaji Python adapter's lifecycle from
// Go: persisting the credentials a user enters in the web UI, and starting/
// stopping adapter/shioaji_adapter.py as a subprocess against them.
package shioajiproc

import (
	"encoding/json"
	"os"
)

// Config holds the Shioaji credentials persisted to config.json. It is
// stored in plaintext next to the executable — a deliberate, documented
// trade-off for now; see the project's credential-storage notes for the
// planned follow-up (OS credential store).
type Config struct {
	APIKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
}

// Load reads Config from path. A missing file is reported as an error
// (os.IsNotExist) rather than an empty Config, so callers can distinguish
// "not configured yet" from "configured with empty values".
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes cfg to path as JSON, creating or overwriting the file.
func Save(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
