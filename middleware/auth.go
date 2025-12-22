package middleware

import (
	"context"
	"errors"
	"strings"

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
