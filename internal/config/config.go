package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type ServiceConfig struct {
	Target       string     `mapstructure:"target" yaml:"target"`
	AuthHeader   string     `mapstructure:"auth_header" yaml:"auth_header"`
	AuthScheme   string     `mapstructure:"auth_scheme" yaml:"auth_scheme"`
	AllowedPaths []string   `mapstructure:"allowed_paths" yaml:"allowed_paths"`
	RateLimit    RateLimit  `mapstructure:"rate_limit" yaml:"rate_limit"`
}

type RateLimit struct {
	RequestsPerMinute int `mapstructure:"requests_per_minute" yaml:"requests_per_minute"`
}

type TimeoutConfig struct {
	Read     int `mapstructure:"read_seconds" yaml:"read_seconds"`
	Write    int `mapstructure:"write_seconds" yaml:"write_seconds"`
	Upstream int `mapstructure:"upstream_seconds" yaml:"upstream_seconds"`
}

type CORSConfig struct {
	Enabled        bool     `mapstructure:"enabled" yaml:"enabled"`
	AllowedOrigins []string `mapstructure:"allowed_origins" yaml:"allowed_origins"`
}

type GatewayConfig struct {
	Enabled bool              `mapstructure:"enabled" yaml:"enabled"`
	Paths   []string          `mapstructure:"paths" yaml:"paths"`
	Models  map[string]string `mapstructure:"models" yaml:"models"`
}

type Config struct {
	Services      map[string]ServiceConfig `mapstructure:"services" yaml:"services"`
	Listen        string                   `mapstructure:"listen" yaml:"listen"`
	ListenUnix    string                   `mapstructure:"listen_unix" yaml:"listen_unix"`
	Timeouts      TimeoutConfig            `mapstructure:"timeouts" yaml:"timeouts"`
	CORS          CORSConfig               `mapstructure:"cors" yaml:"cors"`
	SecretBackend string                   `mapstructure:"secret_backend" yaml:"secret_backend"`
	Gateway       GatewayConfig            `mapstructure:"gateway" yaml:"gateway"`
}

func Load(dir string) (*Config, error) {
	v := viper.New()
	v.SetConfigName("vibeproxy")
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)

	v.SetDefault("listen", "127.0.0.1:8080")
	v.SetDefault("timeouts.read_seconds", 30)
	v.SetDefault("timeouts.write_seconds", 120)
	v.SetDefault("timeouts.upstream_seconds", 90)
	v.SetDefault("cors.enabled", false)
	v.SetDefault("gateway.paths", []string{"/v1/chat/completions"})

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading vibeproxy.yaml: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing vibeproxy.yaml: %w", err)
	}
	return &cfg, nil
}

