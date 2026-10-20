package config

import (
	"errors"
	"fmt"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"strings"
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
	URL     string            `yaml:"url"`
	Timeout Duration          `yaml:"timeout"`
	Headers map[string]string `yaml:"headers"`
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
	if err := c.expandEnv(); err != nil {
		return nil, err
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
	for name, value := range c.Upstream.Headers {
		if err := validateHeader(name, value); err != nil {
			return fmt.Errorf("upstream.headers: %w", err)
		}
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

// envRef matches ${VAR}. Only the braced form is expanded, so a literal "$"
// in a header value is left alone.
var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv replaces ${VAR} in upstream.headers values with the environment
// variable's value, so secrets can stay out of config.yaml. An unset variable
// is an error rather than an empty credential.
func (c *Config) expandEnv() error {
	for name, value := range c.Upstream.Headers {
		var missing []string
		expanded := envRef.ReplaceAllStringFunc(value, func(ref string) string {
			v, ok := os.LookupEnv(envRef.FindStringSubmatch(ref)[1])
			if !ok {
				missing = append(missing, ref)
			}
			return v
		})
		if len(missing) > 0 {
			return fmt.Errorf("upstream.headers.%s: environment variable %s is not set", name, strings.Join(missing, ", "))
		}
		c.Upstream.Headers[name] = expanded
	}
	return nil
}

// reservedHeaders are managed by mcpgate or Go's HTTP client; setting them
// would be ignored or break request framing.
var reservedHeaders = map[string]bool{
	"Host":              true,
	"Content-Length":    true,
	"Transfer-Encoding": true,
	"Connection":        true,
}

func validateHeader(name, value string) error {
	if name == "" || strings.IndexFunc(name, func(r rune) bool { return !isTokenChar(r) }) >= 0 {
		return fmt.Errorf("invalid header name %q", name)
	}
	if reservedHeaders[textproto.CanonicalMIMEHeaderKey(name)] {
		return fmt.Errorf("%s cannot be set", textproto.CanonicalMIMEHeaderKey(name))
	}
	if value == "" {
		return fmt.Errorf("%s: value is empty", name)
	}
	if strings.IndexFunc(value, func(r rune) bool { return (r < ' ' && r != '\t') || r == 0x7f }) >= 0 {
		return fmt.Errorf("%s: value contains invalid characters", name)
	}
	return nil
}

// isTokenChar reports whether r may appear in an HTTP header name (RFC 9110 token).
func isTokenChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", r)
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
