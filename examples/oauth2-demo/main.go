package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	guardian "github.com/grpc-guardian/grpc-guardian"
	"github.com/grpc-guardian/grpc-guardian/middleware"
	pb "github.com/grpc-guardian/grpc-guardian/examples/oauth2-demo/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// server implements the UserService
type server struct {
	pb.UnimplementedUserServiceServer
}

func (s *server) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	// Extract OAuth2 information from context
	userID, _ := ctx.Value("user_id").(string)
	username, _ := ctx.Value("username").(string)
	clientID, _ := ctx.Value("client_id").(string)
	scopes, _ := middleware.GetScopes(ctx)

	log.Printf("GetProfile called by user_id=%s, username=%s, client_id=%s, scopes=%v",
		userID, username, clientID, scopes)

	return &pb.GetProfileResponse{
		UserId:   req.UserId,
		Username: username,
		Email:    fmt.Sprintf("%s@example.com", username),
		Scopes:   scopes,
	}, nil
}

func (s *server) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	username, _ := ctx.Value("username").(string)
	scopes, _ := middleware.GetScopes(ctx)

	log.Printf("UpdateProfile called by username=%s, scopes=%v", username, scopes)

	return &pb.UpdateProfileResponse{
		Success: true,
		Message: "Profile updated successfully",
	}, nil
}

func (s *server) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	username, _ := ctx.Value("username").(string)
	scopes, _ := middleware.GetScopes(ctx)

	log.Printf("ListUsers called by username=%s, scopes=%v", username, scopes)

	// Return mock users
	users := []*pb.User{
		{UserId: "1", Username: "alice", Email: "alice@example.com"},
		{UserId: "2", Username: "bob", Email: "bob@example.com"},
		{UserId: "3", Username: "charlie", Email: "charlie@example.com"},
	}

	return &pb.ListUsersResponse{
		Users: users,
		Total: int32(len(users)),
	}, nil
}

func main() {
	// Mock OAuth2 introspection server
	// In production, this would be your actual OAuth2 provider (Auth0, Keycloak, etc.)
	go startMockOAuth2Server()
	time.Sleep(100 * time.Millisecond) // Wait for mock server to start

	// Configure OAuth2 validator
	oauth2Config := middleware.OAuth2Config{
		IntrospectionURL: "http://localhost:8081/introspect",
		ClientID:         "grpc-guardian-service",
		ClientSecret:     "service-secret",
		Timeout:          5 * time.Second,
	}

	// Create middleware chain with OAuth2 authentication
	chain := guardian.NewChain(
		middleware.Logging(),
		middleware.Auth(middleware.OAuth2Validator(oauth2Config)),
	)

	// Create middleware for methods requiring specific scopes
	// GetProfile: requires "profile:read" scope
	getProfileChain := guardian.NewChain(
		middleware.Logging(),
		middleware.Auth(middleware.OAuth2Validator(oauth2Config)),
		middleware.RequireScope("profile:read"),
	)

	// UpdateProfile: requires "profile:write" scope
	updateProfileChain := guardian.NewChain(
		middleware.Logging(),
		middleware.Auth(middleware.OAuth2Validator(oauth2Config)),
		middleware.RequireScope("profile:write"),
	)

	// ListUsers: requires "users:read" scope
	listUsersChain := guardian.NewChain(
		middleware.Logging(),
		middleware.Auth(middleware.OAuth2Validator(oauth2Config)),
		middleware.RequireScope("users:read"),
	)

	// Create gRPC server with per-method middleware
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			chain.UnaryInterceptor(),
		),
	)

	pb.RegisterUserServiceServer(server, &server{})
	reflection.Register(server)

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Println("gRPC server with OAuth2 authentication listening on :50051")
	log.Println("Mock OAuth2 introspection server running on :8081")
	log.Println("")
	log.Println("Test with grpcurl:")
	log.Println(`  # Valid token with profile:read scope`)
	log.Println(`  grpcurl -plaintext -H "Authorization: Bearer valid-token-profile-read" \`)
	log.Println(`    -d '{"user_id": "123"}' localhost:50051 UserService/GetProfile`)
	log.Println("")
	log.Println(`  # Valid token with profile:write scope`)
	log.Println(`  grpcurl -plaintext -H "Authorization: Bearer valid-token-profile-write" \`)
	log.Println(`    -d '{"user_id": "123", "username": "newname"}' localhost:50051 UserService/UpdateProfile`)
	log.Println("")
	log.Println(`  # Invalid token`)
	log.Println(`  grpcurl -plaintext -H "Authorization: Bearer invalid-token" \`)
	log.Println(`    -d '{"user_id": "123"}' localhost:50051 UserService/GetProfile`)

	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// startMockOAuth2Server starts a mock OAuth2 introspection server for demo purposes
func startMockOAuth2Server() {
	http.HandleFunc("/introspect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check client credentials
		username, password, ok := r.BasicAuth()
		if !ok || username != "grpc-guardian-service" || password != "service-secret" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse token from request body
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		token := r.FormValue("token")
		log.Printf("Introspection request for token: %s", token)

		// Mock token validation
		var response string
		switch token {
		case "valid-token-profile-read":
			// Active token with profile:read scope
			response = `{
				"active": true,
				"scope": "profile:read",
				"client_id": "demo-client",
				"username": "alice",
				"token_type": "Bearer",
				"exp": 1893456000,
				"iat": 1609459200,
				"sub": "user-123",
				"aud": "grpc-guardian-service"
			}`
		case "valid-token-profile-write":
			// Active token with profile:write scope
			response = `{
				"active": true,
				"scope": "profile:write profile:read",
				"client_id": "demo-client",
				"username": "bob",
				"token_type": "Bearer",
				"exp": 1893456000,
				"iat": 1609459200,
				"sub": "user-456",
				"aud": "grpc-guardian-service"
			}`
		case "valid-token-admin":
			// Active token with admin scopes
			response = `{
				"active": true,
				"scope": "profile:read profile:write users:read users:write",
				"client_id": "admin-client",
				"username": "admin",
				"token_type": "Bearer",
				"exp": 1893456000,
				"iat": 1609459200,
				"sub": "admin-001",
				"aud": "grpc-guardian-service"
			}`
		case "expired-token":
			// Inactive token (expired)
			response = `{
				"active": false
			}`
		default:
			// Invalid token
			response = `{
				"active": false
			}`
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	})

	log.Println("Starting mock OAuth2 introspection server on :8081")
	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatalf("Failed to start mock OAuth2 server: %v", err)
	}
}
