package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that unmarshals from a YAML string like "30s".
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }

type RateLimit struct {
	RPS   float64 `yaml:"rps"`
	Burst int     `yaml:"burst"`
}

type Upstream struct {
	URL     string   `yaml:"url"`
	Timeout Duration `yaml:"timeout"`
}

type Key struct {
	Label     string     `yaml:"label"`
	Hash      string     `yaml:"hash"`
	Allow     []string   `yaml:"allow"`
	RateLimit *RateLimit `yaml:"rate_limit"`
}

type Defaults struct {
	RateLimit RateLimit `yaml:"rate_limit"`
}

type Audit struct {
	Output string `yaml:"output"`
}

type Config struct {
	Upstream Upstream `yaml:"upstream"`
	Listen   string   `yaml:"listen"`
	Keys     []Key    `yaml:"keys"`
	Defaults Defaults `yaml:"defaults"`
	Audit    Audit    `yaml:"audit"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &c, nil
}

func (c *Config) Validate() error {
	if c.Upstream.URL == "" {
		return errors.New("upstream.url is required")
	}
	if _, err := url.Parse(c.Upstream.URL); err != nil {
		return fmt.Errorf("upstream.url invalid: %w", err)
	}
	if len(c.Keys) == 0 {
		return errors.New("at least one key is required")
	}
	for i, k := range c.Keys {
		if k.Label == "" {
			return fmt.Errorf("keys[%d]: label is required", i)
		}
		if k.Hash == "" {
			return fmt.Errorf("keys[%d] (%s): hash is required", i, k.Label)
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.Upstream.Timeout.Std() == 0 {
		c.Upstream.Timeout = Duration(30 * time.Second)
	}
	if c.Audit.Output == "" {
		c.Audit.Output = "stdout"
	}
	if c.Defaults.RateLimit.RPS == 0 {
		c.Defaults.RateLimit = RateLimit{RPS: 2, Burst: 5}
	}
	for i := range c.Keys {
		if c.Keys[i].RateLimit == nil {
			rl := c.Defaults.RateLimit
			c.Keys[i].RateLimit = &rl
		}
	}
}
