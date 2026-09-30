// Package config reads the user's pins and caches the repo list between runs.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Named is one saved query or reply template, in the order the user wrote it.
type Named struct{ Name, Value string }

type Config struct {
	Pins      []string
	Queries   []Named
	Templates []Named
}

// Load reads path. A missing file is not an error: it means no pins, queries or templates.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var raw struct {
		Pins      []string  `yaml:"pins"`
		Queries   yaml.Node `yaml:"queries"` // yaml.Node keeps map order; a Go map would not
		Templates yaml.Node `yaml:"templates"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	c := Config{Pins: raw.Pins}
	if c.Queries, err = namedList(raw.Queries, "queries"); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.Templates, err = namedList(raw.Templates, "templates"); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func namedList(n yaml.Node, field string) ([]Named, error) {
	if n.Kind == 0 || n.Tag == "!!null" {
		return nil, nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: want name: value pairs", field)
	}
	var out []Named
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s: %q: value must be text", field, k.Value)
		}
		if strings.TrimSpace(k.Value) == "" || strings.TrimSpace(v.Value) == "" {
			return nil, fmt.Errorf("%s: %q: name and value must not be empty", field, k.Value)
		}
		out = append(out, Named{Name: strings.TrimSpace(k.Value), Value: v.Value})
	}
	return out, nil
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
