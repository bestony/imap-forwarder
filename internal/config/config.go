package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

const (
	DefaultOutputFile   = "mail.txt"
	DefaultPort         = 993
	DefaultMailbox      = "INBOX"
	DefaultTLSMode      = "tls"
	DefaultMaxBodyBytes = 2 * 1024 * 1024
)

// Config is the root application configuration.
type Config struct {
	OutputFile   string    `mapstructure:"output_file"`
	MaxBodyBytes int64     `mapstructure:"max_body_bytes"`
	Accounts     []Account `mapstructure:"accounts"`
}

// Account describes a single IMAP mailbox to pull.
type Account struct {
	Name     string `mapstructure:"name"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Mailbox  string `mapstructure:"mailbox"`
	TLSMode  string `mapstructure:"tls_mode"`
}

// Address returns host:port for dialing.
func (a Account) Address() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

// Load reads and validates configuration from path (typically config.toml).
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")

	v.SetDefault("output_file", DefaultOutputFile)
	v.SetDefault("max_body_bytes", DefaultMaxBodyBytes)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.applyDefaultsAndValidate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyDefaultsAndValidate() error {
	if strings.TrimSpace(c.OutputFile) == "" {
		c.OutputFile = DefaultOutputFile
	}
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if len(c.Accounts) == 0 {
		return fmt.Errorf("config: at least one [[accounts]] entry is required")
	}

	for i := range c.Accounts {
		if err := c.Accounts[i].applyDefaultsAndValidate(i); err != nil {
			return err
		}
	}
	return nil
}

func (a *Account) applyDefaultsAndValidate(index int) error {
	prefix := fmt.Sprintf("accounts[%d]", index)

	if strings.TrimSpace(a.Name) == "" {
		a.Name = fmt.Sprintf("account-%d", index+1)
	}
	if strings.TrimSpace(a.Host) == "" {
		return fmt.Errorf("config: %s.host is required", prefix)
	}
	if a.Port <= 0 {
		a.Port = DefaultPort
	}
	if strings.TrimSpace(a.Username) == "" {
		return fmt.Errorf("config: %s.username is required", prefix)
	}
	if strings.TrimSpace(a.Password) == "" {
		return fmt.Errorf("config: %s.password is required", prefix)
	}
	if strings.TrimSpace(a.Mailbox) == "" {
		a.Mailbox = DefaultMailbox
	}
	if strings.TrimSpace(a.TLSMode) == "" {
		a.TLSMode = DefaultTLSMode
	}

	mode := strings.ToLower(strings.TrimSpace(a.TLSMode))
	switch mode {
	case "tls", "starttls", "insecure":
		a.TLSMode = mode
	default:
		return fmt.Errorf("config: %s.tls_mode must be tls, starttls, or insecure (got %q)", prefix, a.TLSMode)
	}
	return nil
}
