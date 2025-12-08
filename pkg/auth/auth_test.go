package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"
	"time"

	chatv1 "Hermes/gen/chat/v1"

	"github.com/golang-jwt/jwt/v5"
)

// testKey is an RSA key pair for testing
var testKey *rsa.PrivateKey

func init() {
	var err error
	testKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("failed to generate test key: " + err.Error())
	}
}

// setupTestAuth configures auth for testing with a mock key function
func setupTestAuth() {
	SetKeyFunc(func(token *jwt.Token) (interface{}, error) {
		return &testKey.PublicKey, nil
	})
}

// createTestToken creates a JWT token for testing
func createTestToken(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, _ := token.SignedString(testKey)
	return tokenStr
}

func TestUserFromContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		wantUser *chatv1.User
		wantOk   bool
	}{
		{
			name: "user in context",
			ctx: context.WithValue(context.Background(), UserContextKey{}, &chatv1.User{
				Id:       "user-123",
				Username: "testuser",
			}),
			wantUser: &chatv1.User{Id: "user-123", Username: "testuser"},
			wantOk:   true,
		},
		{
			name:     "no user in context",
			ctx:      context.Background(),
			wantUser: nil,
			wantOk:   false,
		},
		{
			name:     "wrong type in context",
			ctx:      context.WithValue(context.Background(), UserContextKey{}, "not a user"),
			wantUser: nil,
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, ok := UserFromContext(tt.ctx)
			if ok != tt.wantOk {
				t.Errorf("UserFromContext() ok = %v, want %v", ok, tt.wantOk)
			}
			if tt.wantOk && (user.Id != tt.wantUser.Id || user.Username != tt.wantUser.Username) {
				t.Errorf("UserFromContext() user = %+v, want %+v", user, tt.wantUser)
			}
		})
	}
}

func TestAuthenticate_NoToken(t *testing.T) {
	setupTestAuth()

	header := http.Header{}
	_, err := Authenticate(header)

	if err != ErrNoToken {
		t.Errorf("Authenticate() error = %v, want %v", err, ErrNoToken)
	}
}

func TestAuthenticate_EmptyBearer(t *testing.T) {
	setupTestAuth()

	header := http.Header{}
	header.Set("Authorization", "")
	_, err := Authenticate(header)

	if err != ErrNoToken {
		t.Errorf("Authenticate() error = %v, want %v", err, ErrNoToken)
	}
}

func TestAuthenticate_ValidToken(t *testing.T) {
	setupTestAuth()

	claims := jwt.MapClaims{
		"iss":              GetCognitoIssuer(),
		"sub":              "user-123",
		"cognito:username": "testuser",
		"exp":              time.Now().Add(time.Hour).Unix(),
	}
	tokenStr := createTestToken(claims)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenStr)

	user, err := Authenticate(header)
	if err != nil {
		t.Fatalf("Authenticate() unexpected error: %v", err)
	}

	if user.Id != "user-123" {
		t.Errorf("user.Id = %q, want %q", user.Id, "user-123")
	}
	if user.Username != "testuser" {
		t.Errorf("user.Username = %q, want %q", user.Username, "testuser")
	}
}

func TestAuthenticate_ValidTokenWithoutBearerPrefix(t *testing.T) {
	setupTestAuth()

	claims := jwt.MapClaims{
		"iss":              GetCognitoIssuer(),
		"sub":              "user-456",
		"cognito:username": "anotheruser",
		"exp":              time.Now().Add(time.Hour).Unix(),
	}
	tokenStr := createTestToken(claims)

	header := http.Header{}
	header.Set("Authorization", tokenStr)

	user, err := Authenticate(header)
	if err != nil {
		t.Fatalf("Authenticate() unexpected error: %v", err)
	}

	if user.Id != "user-456" {
		t.Errorf("user.Id = %q, want %q", user.Id, "user-456")
	}
}

func TestAuthenticate_ExpiredToken(t *testing.T) {
	setupTestAuth()

	claims := jwt.MapClaims{
		"iss":              GetCognitoIssuer(),
		"sub":              "user-123",
		"cognito:username": "testuser",
		"exp":              time.Now().Add(-time.Hour).Unix(), // expired
	}
	tokenStr := createTestToken(claims)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenStr)

	_, err := Authenticate(header)
	if err == nil {
		t.Error("Authenticate() expected error for expired token")
	}
}

func TestAuthenticate_InvalidSignature(t *testing.T) {
	// Create a different key
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	setupTestAuth()

	claims := jwt.MapClaims{
		"iss":              GetCognitoIssuer(),
		"sub":              "user-123",
		"cognito:username": "testuser",
		"exp":              time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, _ := token.SignedString(otherKey) // Sign with different key

	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokenStr)

	_, err := Authenticate(header)
	if err == nil {
		t.Error("Authenticate() expected error for invalid signature")
	}
}

func TestParseClaims_ValidClaims(t *testing.T) {
	claims := jwt.MapClaims{
		"iss":              GetCognitoIssuer(),
		"sub":              "user-789",
		"cognito:username": "parseuser",
	}

	user, err := ParseClaims(claims)
	if err != nil {
		t.Fatalf("ParseClaims() unexpected error: %v", err)
	}

	if user.Id != "user-789" {
		t.Errorf("user.Id = %q, want %q", user.Id, "user-789")
	}
	if user.Username != "parseuser" {
		t.Errorf("user.Username = %q, want %q", user.Username, "parseuser")
	}
}

func TestParseClaims_MissingSub(t *testing.T) {
	claims := jwt.MapClaims{
		"iss":              GetCognitoIssuer(),
		"cognito:username": "nosubuser",
	}

	_, err := ParseClaims(claims)
	if err == nil {
		t.Error("ParseClaims() expected error for missing sub claim")
	}
}

func TestParseClaims_InvalidIssuer(t *testing.T) {
	claims := jwt.MapClaims{
		"iss":              "https://wrong-issuer.com",
		"sub":              "user-123",
		"cognito:username": "testuser",
	}

	_, err := ParseClaims(claims)
	if err == nil {
		t.Error("ParseClaims() expected error for invalid issuer")
	}
}

func TestParseClaims_MissingUsername(t *testing.T) {
	claims := jwt.MapClaims{
		"iss": GetCognitoIssuer(),
		"sub": "user-no-username",
	}

	user, err := ParseClaims(claims)
	if err != nil {
		t.Fatalf("ParseClaims() unexpected error: %v", err)
	}

	if user.Id != "user-no-username" {
		t.Errorf("user.Id = %q, want %q", user.Id, "user-no-username")
	}
	if user.Username != "" {
		t.Errorf("user.Username = %q, want empty string", user.Username)
	}
}

func TestGetCognitoIssuer(t *testing.T) {
	issuer := GetCognitoIssuer()
	if issuer == "" {
		t.Error("GetCognitoIssuer() returned empty string")
	}
	// Should contain the expected format
	expected := "https://cognito-idp."
	if len(issuer) < len(expected) || issuer[:len(expected)] != expected {
		t.Errorf("GetCognitoIssuer() = %q, should start with %q", issuer, expected)
	}
}

func TestNewAuthInterceptor(t *testing.T) {
	interceptor := NewAuthInterceptor()
	if interceptor == nil {
		t.Error("NewAuthInterceptor() returned nil")
	}
}

func TestNewStreamingAuthInterceptor(t *testing.T) {
	interceptor := NewStreamingAuthInterceptor()
	if interceptor == nil {
		t.Error("NewStreamingAuthInterceptor() returned nil")
	}
}
