// Package config reads the user's pins and caches the repo list between runs.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Pins []string `yaml:"pins"`
}

// Load reads path. A missing file is not an error: it means no pins.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// ReadRepoCache returns the cached repo names, or nil if there is no usable cache.
func ReadRepoCache(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var names []string
	if json.Unmarshal(b, &names) != nil {
		return nil
	}
	return names
}

func WriteRepoCache(path string, names []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(names)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
