package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthValidator defines the interface for authentication validation
type AuthValidator func(ctx context.Context, token string) (context.Context, error)

// Auth creates an authentication middleware with the provided validator
//
// Example usage:
//
//	// JWT authentication
//	chain := guardian.NewChain(
//	    middleware.Auth(middleware.JWTValidator("your-secret-key")),
//	)
//
//	// API key authentication
//	chain := guardian.NewChain(
//	    middleware.Auth(middleware.APIKeyValidator(func(key string) bool {
//	        return key == "valid-api-key"
//	    })),
//	)
func Auth(validator AuthValidator) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract token from metadata
		token, err := extractToken(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated,
				"missing or invalid authentication token: %v\nHint: Include 'authorization: Bearer <token>' or 'x-api-key: <key>' in gRPC metadata", err)
		}

		// Validate token
		ctx, err = validator(ctx, token)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated,
				"authentication failed: %v\nHint: Verify token format, expiration, and signing key", err)
		}

		// Call next handler
		return handler(ctx, req)
	}
}

// JWTValidator creates a JWT token validator
func JWTValidator(secret string) AuthValidator {
	return func(ctx context.Context, tokenString string) (context.Context, error) {
		// Parse JWT token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("invalid signing method")
			}
			return []byte(secret), nil
		})

		if err != nil {
			return ctx, err
		}

		if !token.Valid {
			return ctx, errors.New("invalid token")
		}

		// Extract claims and add to context
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// Add user ID to context
			if userID, ok := claims["sub"].(string); ok {
				ctx = context.WithValue(ctx, "user_id", userID)
			}

			// Add roles to context
			if roles, ok := claims["roles"].([]interface{}); ok {
				roleStrings := make([]string, len(roles))
				for i, role := range roles {
					if roleStr, ok := role.(string); ok {
						roleStrings[i] = roleStr
					}
				}
				ctx = context.WithValue(ctx, "roles", roleStrings)
			}
		}

		return ctx, nil
	}
}

// APIKeyValidator creates an API key validator
func APIKeyValidator(isValidKey func(string) bool) AuthValidator {
	return func(ctx context.Context, apiKey string) (context.Context, error) {
		if !isValidKey(apiKey) {
			return ctx, errors.New("invalid API key")
		}

		// Add API key to context
		ctx = context.WithValue(ctx, "api_key", apiKey)
		return ctx, nil
	}
}

// BasicAuthValidator creates a basic authentication validator
func BasicAuthValidator(username, password string) AuthValidator {
	return func(ctx context.Context, token string) (context.Context, error) {
		// Parse basic auth format: "username:password"
		parts := strings.SplitN(token, ":", 2)
		if len(parts) != 2 {
			return ctx, errors.New("invalid basic auth format")
		}

		if parts[0] != username || parts[1] != password {
			return ctx, errors.New("invalid credentials")
		}

		ctx = context.WithValue(ctx, "username", username)
		return ctx, nil
	}
}

// OAuth2Config holds the configuration for OAuth 2.0 token introspection
type OAuth2Config struct {
	// IntrospectionURL is the OAuth 2.0 introspection endpoint (RFC 7662)
	IntrospectionURL string

	// ClientID is the client identifier for introspection authentication
	ClientID string

	// ClientSecret is the client secret for introspection authentication
	ClientSecret string

	// HTTPClient is the HTTP client to use for introspection requests
	// If nil, http.DefaultClient will be used
	HTTPClient *http.Client

	// Timeout is the timeout for introspection requests
	// Default: 5 seconds
	Timeout time.Duration
}

// oauth2IntrospectionResponse represents the response from an OAuth 2.0 introspection endpoint
// as defined in RFC 7662
type oauth2IntrospectionResponse struct {
	// Active indicates whether the token is currently active
	Active bool `json:"active"`

	// Scope is a space-separated list of scopes associated with the token
	Scope string `json:"scope,omitempty"`

	// ClientID is the client identifier for the OAuth 2.0 client
	ClientID string `json:"client_id,omitempty"`

	// Username is the human-readable identifier for the resource owner
	Username string `json:"username,omitempty"`

	// TokenType is the type of the token (e.g., "Bearer")
	TokenType string `json:"token_type,omitempty"`

	// Exp is the expiration timestamp (seconds since epoch)
	Exp int64 `json:"exp,omitempty"`

	// Iat is the issued-at timestamp (seconds since epoch)
	Iat int64 `json:"iat,omitempty"`

	// Nbf is the not-before timestamp (seconds since epoch)
	Nbf int64 `json:"nbf,omitempty"`

	// Sub is the subject of the token (usually user ID)
	Sub string `json:"sub,omitempty"`

	// Aud is the intended audience of the token
	Aud string `json:"aud,omitempty"`

	// Iss is the issuer of the token
	Iss string `json:"iss,omitempty"`

	// Jti is the unique identifier for the token
	Jti string `json:"jti,omitempty"`
}

// OAuth2Validator creates an OAuth 2.0 token validator using introspection endpoint
//
// This validator implements RFC 7662 (OAuth 2.0 Token Introspection).
// It validates access tokens by calling the introspection endpoint of the OAuth 2.0 provider.
//
// Example usage:
//
//	config := middleware.OAuth2Config{
//	    IntrospectionURL: "https://oauth-provider.com/introspect",
//	    ClientID:         "my-client-id",
//	    ClientSecret:     "my-client-secret",
//	    Timeout:          5 * time.Second,
//	}
//	chain := guardian.NewChain(
//	    middleware.Auth(middleware.OAuth2Validator(config)),
//	)
func OAuth2Validator(config OAuth2Config) AuthValidator {
	// Set defaults
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	return func(ctx context.Context, token string) (context.Context, error) {
		// Create introspection request
		data := url.Values{}
		data.Set("token", token)

		req, err := http.NewRequestWithContext(ctx, "POST", config.IntrospectionURL, strings.NewReader(data.Encode()))
		if err != nil {
			return ctx, fmt.Errorf("failed to create introspection request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(config.ClientID, config.ClientSecret)

		// Apply timeout
		reqCtx, cancel := context.WithTimeout(ctx, config.Timeout)
		defer cancel()
		req = req.WithContext(reqCtx)

		// Send introspection request
		resp, err := config.HTTPClient.Do(req)
		if err != nil {
			return ctx, fmt.Errorf("introspection request failed: %w", err)
		}
		defer resp.Body.Close()

		// Check HTTP status
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return ctx, fmt.Errorf("introspection endpoint returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse introspection response
		var introspectionResp oauth2IntrospectionResponse
		if err := json.NewDecoder(resp.Body).Decode(&introspectionResp); err != nil {
			return ctx, fmt.Errorf("failed to parse introspection response: %w", err)
		}

		// Check if token is active
		if !introspectionResp.Active {
			return ctx, errors.New("token is not active")
		}

		// Add claims to context
		if introspectionResp.Sub != "" {
			ctx = context.WithValue(ctx, "user_id", introspectionResp.Sub)
		}
		if introspectionResp.Username != "" {
			ctx = context.WithValue(ctx, "username", introspectionResp.Username)
		}
		if introspectionResp.ClientID != "" {
			ctx = context.WithValue(ctx, "client_id", introspectionResp.ClientID)
		}
		if introspectionResp.Scope != "" {
			// Split space-separated scopes into array
			scopes := strings.Fields(introspectionResp.Scope)
			ctx = context.WithValue(ctx, "scopes", scopes)
		}

		// Store full introspection response in context for advanced use cases
		ctx = context.WithValue(ctx, "oauth2_introspection", introspectionResp)

		return ctx, nil
	}
}

// RequireRole creates middleware that requires specific roles
func RequireRole(requiredRoles ...string) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Get roles from context
		roles, ok := ctx.Value("roles").([]string)
		if !ok {
			return nil, status.Error(codes.PermissionDenied,
				"no roles found in context\n"+
				"Hint: Ensure user is authenticated with a JWT token containing 'roles' claim, "+
				"or use RequireRole middleware after Auth middleware")
		}

		// Check if user has required role
		hasRole := false
		for _, userRole := range roles {
			for _, requiredRole := range requiredRoles {
				if userRole == requiredRole {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}

		if !hasRole {
			return nil, status.Errorf(codes.PermissionDenied,
				"insufficient permissions: requires one of %v, user has %v", requiredRoles, roles)
		}

		return handler(ctx, req)
	}
}

// RequireScope creates middleware that requires specific OAuth 2.0 scopes
func RequireScope(requiredScopes ...string) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Get scopes from context
		scopes, ok := ctx.Value("scopes").([]string)
		if !ok {
			return nil, status.Error(codes.PermissionDenied,
				"no scopes found in context\n"+
					"Hint: Ensure user is authenticated with an OAuth 2.0 token, "+
					"or use RequireScope middleware after Auth middleware with OAuth2Validator")
		}

		// Check if user has required scope
		hasScope := false
		for _, userScope := range scopes {
			for _, requiredScope := range requiredScopes {
				if userScope == requiredScope {
					hasScope = true
					break
				}
			}
			if hasScope {
				break
			}
		}

		if !hasScope {
			return nil, status.Errorf(codes.PermissionDenied,
				"insufficient permissions: requires one of scopes %v, user has %v", requiredScopes, scopes)
		}

		return handler(ctx, req)
	}
}

// extractToken extracts the authentication token from the gRPC metadata
func extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata found")
	}

	// Try to get from "authorization" header
	authHeaders := md.Get("authorization")
	if len(authHeaders) > 0 {
		// Remove "Bearer " prefix if present
		token := authHeaders[0]
		if strings.HasPrefix(token, "Bearer ") {
			return strings.TrimPrefix(token, "Bearer "), nil
		}
		return token, nil
	}

	// Try to get from "x-api-key" header
	apiKeyHeaders := md.Get("x-api-key")
	if len(apiKeyHeaders) > 0 {
		return apiKeyHeaders[0], nil
	}

	return "", errors.New("no authentication token found")
}

// GetUserID retrieves the user ID from context
func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value("user_id").(string)
	return userID, ok
}

// GetRoles retrieves the roles from context
func GetRoles(ctx context.Context) ([]string, bool) {
	roles, ok := ctx.Value("roles").([]string)
	return roles, ok
}

// GetScopes retrieves the OAuth 2.0 scopes from context
func GetScopes(ctx context.Context) ([]string, bool) {
	scopes, ok := ctx.Value("scopes").([]string)
	return scopes, ok
}

// GetClientID retrieves the OAuth 2.0 client ID from context
func GetClientID(ctx context.Context) (string, bool) {
	clientID, ok := ctx.Value("client_id").(string)
	return clientID, ok
}

// Error helper functions for better error messages

// ErrMissingToken creates a detailed error for missing authentication tokens
func ErrMissingToken() error {
	return status.Error(codes.Unauthenticated,
		"authentication token not found\n"+
			"Hint: Include one of the following in gRPC metadata:\n"+
			"  - 'authorization: Bearer <jwt-token>'\n"+
			"  - 'x-api-key: <api-key>'")
}

// ErrInvalidToken creates a detailed error for invalid tokens
func ErrInvalidToken(reason string) error {
	return status.Errorf(codes.Unauthenticated,
		"authentication token is invalid: %s\n"+
			"Hint: Verify token format, expiration, and signing key", reason)
}

// ErrInsufficientPermissions creates a detailed error for permission denials
func ErrInsufficientPermissions(required, actual []string) error {
	return status.Errorf(codes.PermissionDenied,
		"insufficient permissions\n"+
			"Required: %v\n"+
			"Actual: %v\n"+
			"Hint: Contact your administrator to request the required roles", required, actual)
}

// ErrNoRolesInContext creates a detailed error for missing roles in context
func ErrNoRolesInContext() error {
	return status.Error(codes.PermissionDenied,
		"no roles found in context\n"+
			"Hint: Ensure Auth middleware is applied before RequireRole middleware\n"+
			"Example: guardian.NewChain(middleware.Auth(...), middleware.RequireRole(...))")
}
