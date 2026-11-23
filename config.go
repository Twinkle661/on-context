package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Duration lets config.json say "2h" / "90m" instead of nanoseconds.
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"2h\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

type Config struct {
	Listen   string   `json:"listen"`   // e.g. ":8080"
	DataFile string   `json:"dataFile"` // e.g. "/data/notes.json"
	Token    string   `json:"token"`    // bearer token
	TTL      Duration `json:"ttl"`      // non-persistent notes older than this are dropped
}

// loadConfig: defaults < config.json < .env file < process environment.
func loadConfig(path string) (Config, error) {
	cfg := Config{
		Listen:   ":8080",
		DataFile: "notes.json",
		TTL:      Duration{2 * time.Hour},
	}

	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}

	if err := loadDotEnv(".env"); err != nil {
		return cfg, err
	}
	if v := os.Getenv("ONCONTEXT_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("ONCONTEXT_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("ONCONTEXT_DATA"); v != "" {
		cfg.DataFile = v
	}
	if v := os.Getenv("ONCONTEXT_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("ONCONTEXT_TTL: %w", err)
		}
		cfg.TTL.Duration = d
	}

	if cfg.Token == "" {
		return cfg, errors.New("no token configured (set ONCONTEXT_TOKEN in .env / env, or \"token\" in config.json)")
	}
	if cfg.TTL.Duration <= 0 {
		return cfg, errors.New("ttl must be positive")
	}
	return cfg, nil
}

// loadDotEnv reads KEY=VALUE lines into the environment without overriding
// variables that are already set. A missing file is not an error.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
	return sc.Err()
}
