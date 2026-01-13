package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestOAuth2Validator(t *testing.T) {
	// Create mock OAuth2 introspection server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Verify content type
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			http.Error(w, "Invalid content type", http.StatusBadRequest)
			return
		}

		// Verify client credentials
		username, password, ok := r.BasicAuth()
		if !ok || username != "client-id" || password != "client-secret" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse form
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		token := r.FormValue("token")

		// Mock responses based on token
		var response interface{}
		switch token {
		case "valid-token":
			response = map[string]interface{}{
				"active":    true,
				"scope":     "read write",
				"client_id": "test-client",
				"username":  "testuser",
				"sub":       "user-123",
				"exp":       time.Now().Add(1 * time.Hour).Unix(),
			}
		case "expired-token":
			response = map[string]interface{}{
				"active": false,
			}
		case "no-scope-token":
			response = map[string]interface{}{
				"active":    true,
				"client_id": "test-client",
				"sub":       "user-456",
			}
		default:
			response = map[string]interface{}{
				"active": false,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	tests := []struct {
		name          string
		token         string
		wantErr       bool
		wantCode      codes.Code
		validateCtx   func(t *testing.T, ctx context.Context)
	}{
		{
			name:    "valid token",
			token:   "valid-token",
			wantErr: false,
			validateCtx: func(t *testing.T, ctx context.Context) {
				// Check context values
				if userID, ok := ctx.Value("user_id").(string); !ok || userID != "user-123" {
					t.Errorf("Expected user_id=user-123, got %v", userID)
				}
				if username, ok := ctx.Value("username").(string); !ok || username != "testuser" {
					t.Errorf("Expected username=testuser, got %v", username)
				}
				if clientID, ok := ctx.Value("client_id").(string); !ok || clientID != "test-client" {
					t.Errorf("Expected client_id=test-client, got %v", clientID)
				}
				if scopes, ok := ctx.Value("scopes").([]string); !ok || len(scopes) != 2 {
					t.Errorf("Expected 2 scopes, got %v", scopes)
				}
			},
		},
		{
			name:     "invalid token",
			token:    "invalid-token",
			wantErr:  true,
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "expired token",
			token:    "expired-token",
			wantErr:  true,
			wantCode: codes.Unauthenticated,
		},
		{
			name:    "token without scopes",
			token:   "no-scope-token",
			wantErr: false,
			validateCtx: func(t *testing.T, ctx context.Context) {
				// Check context values
				if userID, ok := ctx.Value("user_id").(string); !ok || userID != "user-456" {
					t.Errorf("Expected user_id=user-456, got %v", userID)
				}
				// Scopes should be empty
				if scopes, ok := ctx.Value("scopes").([]string); ok && len(scopes) != 0 {
					t.Errorf("Expected no scopes, got %v", scopes)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create OAuth2 config
			config := OAuth2Config{
				IntrospectionURL: mockServer.URL,
				ClientID:         "client-id",
				ClientSecret:     "client-secret",
				Timeout:          5 * time.Second,
			}

			validator := OAuth2Validator(config)

			// Create context with metadata
			md := metadata.Pairs("authorization", "Bearer "+tt.token)
			ctx := metadata.NewIncomingContext(context.Background(), md)

			// Call validator
			newCtx, err := validator(ctx, tt.token)

			if (err != nil) != tt.wantErr {
				t.Errorf("OAuth2Validator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if st, ok := status.FromError(err); ok {
					if st.Code() != tt.wantCode {
						t.Errorf("Expected code %v, got %v", tt.wantCode, st.Code())
					}
				}
			} else if tt.validateCtx != nil {
				tt.validateCtx(t, newCtx)
			}
		})
	}
}

func TestOAuth2ValidatorHTTPErrors(t *testing.T) {
	tests := []struct {
		name         string
		serverStatus int
		serverBody   string
		wantErr      bool
	}{
		{
			name:         "server returns 500",
			serverStatus: http.StatusInternalServerError,
			serverBody:   "Internal Server Error",
			wantErr:      true,
		},
		{
			name:         "server returns 401",
			serverStatus: http.StatusUnauthorized,
			serverBody:   "Unauthorized",
			wantErr:      true,
		},
		{
			name:         "server returns invalid JSON",
			serverStatus: http.StatusOK,
			serverBody:   "invalid json",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverBody))
			}))
			defer mockServer.Close()

			config := OAuth2Config{
				IntrospectionURL: mockServer.URL,
				ClientID:         "client-id",
				ClientSecret:     "client-secret",
				Timeout:          5 * time.Second,
			}

			validator := OAuth2Validator(config)

			ctx := context.Background()
			_, err := validator(ctx, "test-token")

			if (err != nil) != tt.wantErr {
				t.Errorf("OAuth2Validator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOAuth2ValidatorTimeout(t *testing.T) {
	// Create a slow mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	config := OAuth2Config{
		IntrospectionURL: mockServer.URL,
		ClientID:         "client-id",
		ClientSecret:     "client-secret",
		Timeout:          100 * time.Millisecond, // Short timeout
	}

	validator := OAuth2Validator(config)

	ctx := context.Background()
	_, err := validator(ctx, "test-token")

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestRequireScope(t *testing.T) {
	tests := []struct {
		name           string
		userScopes     []string
		requiredScopes []string
		wantErr        bool
		wantCode       codes.Code
	}{
		{
			name:           "user has required scope",
			userScopes:     []string{"read", "write"},
			requiredScopes: []string{"read"},
			wantErr:        false,
		},
		{
			name:           "user has one of required scopes",
			userScopes:     []string{"read", "write"},
			requiredScopes: []string{"admin", "write"},
			wantErr:        false,
		},
		{
			name:           "user missing required scope",
			userScopes:     []string{"read"},
			requiredScopes: []string{"write"},
			wantErr:        true,
			wantCode:       codes.PermissionDenied,
		},
		{
			name:           "no scopes in context",
			userScopes:     nil,
			requiredScopes: []string{"read"},
			wantErr:        true,
			wantCode:       codes.PermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create context with scopes
			ctx := context.Background()
			if tt.userScopes != nil {
				ctx = context.WithValue(ctx, "scopes", tt.userScopes)
			}

			// Create middleware
			handler := RequireScope(tt.requiredScopes...)

			// Mock handler
			mockHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
				return "success", nil
			}

			// Call middleware
			_, err := handler(ctx, nil, &grpc.UnaryServerInfo{}, mockHandler)

			if (err != nil) != tt.wantErr {
				t.Errorf("RequireScope() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if st, ok := status.FromError(err); ok {
					if st.Code() != tt.wantCode {
						t.Errorf("Expected code %v, got %v", tt.wantCode, st.Code())
					}
				}
			}
		})
	}
}

func TestGetScopes(t *testing.T) {
	tests := []struct {
		name       string
		ctx        context.Context
		wantScopes []string
		wantOK     bool
	}{
		{
			name:       "scopes present",
			ctx:        context.WithValue(context.Background(), "scopes", []string{"read", "write"}),
			wantScopes: []string{"read", "write"},
			wantOK:     true,
		},
		{
			name:       "scopes not present",
			ctx:        context.Background(),
			wantScopes: nil,
			wantOK:     false,
		},
		{
			name:       "wrong type in context",
			ctx:        context.WithValue(context.Background(), "scopes", "not-a-slice"),
			wantScopes: nil,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScopes, gotOK := GetScopes(tt.ctx)

			if gotOK != tt.wantOK {
				t.Errorf("GetScopes() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if tt.wantOK {
				if len(gotScopes) != len(tt.wantScopes) {
					t.Errorf("GetScopes() scopes = %v, want %v", gotScopes, tt.wantScopes)
				}
			}
		})
	}
}

func TestGetClientID(t *testing.T) {
	tests := []struct {
		name         string
		ctx          context.Context
		wantClientID string
		wantOK       bool
	}{
		{
			name:         "client_id present",
			ctx:          context.WithValue(context.Background(), "client_id", "test-client"),
			wantClientID: "test-client",
			wantOK:       true,
		},
		{
			name:         "client_id not present",
			ctx:          context.Background(),
			wantClientID: "",
			wantOK:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotClientID, gotOK := GetClientID(tt.ctx)

			if gotOK != tt.wantOK {
				t.Errorf("GetClientID() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if gotClientID != tt.wantClientID {
				t.Errorf("GetClientID() clientID = %v, want %v", gotClientID, tt.wantClientID)
			}
		})
	}
}
