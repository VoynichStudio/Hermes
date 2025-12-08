// Package config provides centralized configuration management
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all application configuration
type Config struct {
	// Server settings
	Server ServerConfig

	// AWS Cognito settings
	Cognito CognitoConfig

	// Logging settings
	Log LogConfig
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Host string
	Port int
}

// Address returns the server address in host:port format
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// CognitoConfig holds AWS Cognito configuration
type CognitoConfig struct {
	Region     string
	UserPoolID string
}

// IssuerURL returns the Cognito issuer URL
func (c CognitoConfig) IssuerURL() string {
	return fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", c.Region, c.UserPoolID)
}

// JWKSURL returns the Cognito JWKS URL
func (c CognitoConfig) JWKSURL() string {
	return c.IssuerURL() + "/.well-known/jwks.json"
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string // "json" or "text"
}

// Load reads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "localhost"),
			Port: getEnvInt("SERVER_PORT", 8080),
		},
		Cognito: CognitoConfig{
			Region:     getEnv("COGNITO_REGION", "us-east-1"),
			UserPoolID: getEnv("COGNITO_USER_POOL_ID", "us-east-1_example"),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "text"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// Validate checks that required configuration is present
func (c *Config) Validate() error {
	if c.Cognito.UserPoolID == "" || c.Cognito.UserPoolID == "us-east-1_example" {
		return fmt.Errorf("COGNITO_USER_POOL_ID should be set to a real value")
	}
	return nil
}

// getEnv returns an environment variable value or a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns an environment variable as int or a default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
