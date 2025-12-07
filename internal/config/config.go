package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig
	Cognito CognitoConfig
	Log     LogConfig
}

type ServerConfig struct {
	Host       string
	Port       string
	TLSEnabled bool
}

type CognitoConfig struct {
	Region     string
	UserPoolID string
}

type LogConfig struct {
	Level  string
	Format string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set default values
	setServerDefaults(v)
	setCognitoDefaults(v)
	setLogDefaults(v)

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

func setServerDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "localhost")
	v.SetDefault("server.port", "8080")
	v.SetDefault("server.tlsenabled", false)
}

func setCognitoDefaults(v *viper.Viper) {
	v.SetDefault("cognito.region", "")
	v.SetDefault("cognito.userpoolid", "")
}

func setLogDefaults(v *viper.Viper) {
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
}
