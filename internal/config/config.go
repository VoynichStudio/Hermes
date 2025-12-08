package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

const (
	configName = "config"
	configType = "yaml"
	configPath = "./configs"
)

// Config represents the application configuration
// Note: Field names must be exported (start with capital letter) for Viper to work properly
type Config struct {
	Server  ServerConfig  `mapstructure:"server" validate:"required"`
	Cognito CognitoConfig `mapstructure:"cognito"`
	Log     LogConfig     `mapstructure:"log"`
	DB      DBConfig      `mapstructure:"db"`
	Redis   RedisConfig   `mapstructure:"redis"`
}

type ServerConfig struct {
	Host       string `mapstructure:"host" validate:"required,hostname|ip"`
	Port       string `mapstructure:"port" validate:"required,number"`
	TLSEnabled bool   `mapstructure:"tls_enabled"`
}

type CognitoConfig struct {
	Region     string `mapstructure:"region" validate:"required_with=UserPoolID"`
	UserPoolID string `mapstructure:"user_pool_id"`
}

type LogConfig struct {
	Level  string `mapstructure:"level" validate:"oneof=debug info warn error fatal panic"`
	Format string `mapstructure:"format" validate:"oneof=json text"`
}

type DBConfig struct {
	// Add your database configuration fields here
}

type RedisConfig struct {
	// Add your Redis configuration fields here
}

// LoadConfig loads configuration from file and environment variables.
// Environment variables take precedence over file configuration.
func LoadConfig() (*Config, error) {
	v := viper.New()

	// Configure viper
	v.SetConfigName(configName)
	v.SetConfigType(configType)
	v.AddConfigPath(configPath)
	v.AddConfigPath(".")

	// Enable environment variable support
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set default values
	setServerDefaults(v)
	setCognitoDefaults(v)
	setLogDefaults(v)

	// Read config file if it exists
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, but we can continue with defaults and env vars
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// validateConfig validates the configuration using struct tags
func validateConfig(cfg *Config) error {
	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		var invalidValidationError *validator.InvalidValidationError
		if errors.As(err, &invalidValidationError) {
			return fmt.Errorf("validation error: %w", err)
		}

		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			errMsg := "validation failed:\n"
			for _, e := range validationErrs {
				errMsg += fmt.Sprintf("- %s: %s\n", e.Field(), e.Tag())
			}
			return fmt.Errorf(errMsg)
		}
		return err
	}

	return nil
}

func setServerDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", "8080")
	v.SetDefault("server.tls_enabled", false)
}

func setCognitoDefaults(v *viper.Viper) {
	v.SetDefault("cognito.region", "")
	v.SetDefault("cognito.user_pool_id", "")
}

func setLogDefaults(v *viper.Viper) {
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
}

// GetConfigPath returns the path to the config file
func GetConfigPath() string {
	// First check the current directory
	if _, err := os.Stat(filepath.Join(".", configName+"."+configType)); err == nil {
		return "./" + configName + "." + configType
	}

	// Then check the configs directory
	return filepath.Join(configPath, configName+"."+configType)
}
