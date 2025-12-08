package config

import (
	"testing"
)

func TestServerConfig_Address(t *testing.T) {
	tests := []struct {
		name     string
		config   ServerConfig
		expected string
	}{
		{
			name:     "default values",
			config:   ServerConfig{Host: "localhost", Port: 8080},
			expected: "localhost:8080",
		},
		{
			name:     "custom host and port",
			config:   ServerConfig{Host: "0.0.0.0", Port: 3000},
			expected: "0.0.0.0:3000",
		},
		{
			name:     "empty host",
			config:   ServerConfig{Host: "", Port: 8080},
			expected: ":8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.Address()
			if result != tt.expected {
				t.Errorf("Address() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCognitoConfig_IssuerURL(t *testing.T) {
	tests := []struct {
		name     string
		config   CognitoConfig
		expected string
	}{
		{
			name:     "us-east-1 region",
			config:   CognitoConfig{Region: "us-east-1", UserPoolID: "us-east-1_ABC123"},
			expected: "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_ABC123",
		},
		{
			name:     "eu-west-1 region",
			config:   CognitoConfig{Region: "eu-west-1", UserPoolID: "eu-west-1_XYZ789"},
			expected: "https://cognito-idp.eu-west-1.amazonaws.com/eu-west-1_XYZ789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IssuerURL()
			if result != tt.expected {
				t.Errorf("IssuerURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCognitoConfig_JWKSURL(t *testing.T) {
	config := CognitoConfig{Region: "us-east-1", UserPoolID: "us-east-1_ABC123"}
	expected := "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_ABC123/.well-known/jwks.json"

	result := config.JWKSURL()
	if result != expected {
		t.Errorf("JWKSURL() = %q, want %q", result, expected)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantError bool
	}{
		{
			name: "valid config",
			config: Config{
				Cognito: CognitoConfig{UserPoolID: "us-east-1_RealPool"},
			},
			wantError: false,
		},
		{
			name: "empty user pool ID",
			config: Config{
				Cognito: CognitoConfig{UserPoolID: ""},
			},
			wantError: true,
		},
		{
			name: "example user pool ID",
			config: Config{
				Cognito: CognitoConfig{UserPoolID: "us-east-1_example"},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	// Set environment variables for testing
	t.Setenv("SERVER_HOST", "0.0.0.0")
	t.Setenv("SERVER_PORT", "3000")
	t.Setenv("COGNITO_REGION", "eu-west-1")
	t.Setenv("COGNITO_USER_POOL_ID", "eu-west-1_TestPool")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 3000)
	}
	if cfg.Cognito.Region != "eu-west-1" {
		t.Errorf("Cognito.Region = %q, want %q", cfg.Cognito.Region, "eu-west-1")
	}
	if cfg.Cognito.UserPoolID != "eu-west-1_TestPool" {
		t.Errorf("Cognito.UserPoolID = %q, want %q", cfg.Cognito.UserPoolID, "eu-west-1_TestPool")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
}

func TestLoad_WithDefaults(t *testing.T) {
	// Don't set any env vars, Load should use defaults but fail validation
	cfg, err := Load()

	// Should return error due to default user pool ID
	if err == nil {
		t.Error("Load() expected validation error for default user pool ID")
	}

	// Config should still be returned with defaults
	if cfg.Server.Host != "localhost" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "localhost")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
}

func TestLoad_InvalidPortFallsBackToDefault(t *testing.T) {
	t.Setenv("SERVER_PORT", "not-a-number")
	t.Setenv("COGNITO_USER_POOL_ID", "us-east-1_ValidPool")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d (default)", cfg.Server.Port, 8080)
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("TEST_VAR", "test_value")

	result := getEnv("TEST_VAR", "default")
	if result != "test_value" {
		t.Errorf("getEnv() = %q, want %q", result, "test_value")
	}

	result = getEnv("NONEXISTENT_VAR", "default")
	if result != "default" {
		t.Errorf("getEnv() = %q, want %q", result, "default")
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue int
		expected     int
	}{
		{
			name:         "valid integer",
			envKey:       "TEST_INT",
			envValue:     "42",
			defaultValue: 0,
			expected:     42,
		},
		{
			name:         "invalid integer uses default",
			envKey:       "TEST_INT_INVALID",
			envValue:     "not-int",
			defaultValue: 100,
			expected:     100,
		},
		{
			name:         "empty uses default",
			envKey:       "TEST_INT_EMPTY",
			envValue:     "",
			defaultValue: 50,
			expected:     50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.envKey, tt.envValue)
			}
			result := getEnvInt(tt.envKey, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvInt() = %d, want %d", result, tt.expected)
			}
		})
	}
}
