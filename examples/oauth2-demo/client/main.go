package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/grpc-guardian/grpc-guardian/examples/oauth2-demo/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func main() {
	// Connect to server
	conn, err := grpc.Dial("localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewUserServiceClient(conn)

	fmt.Println("=== OAuth2 Authentication Demo ===\n")

	// Test 1: Valid token with profile:read scope
	fmt.Println("1. GetProfile with valid token (profile:read scope)")
	testGetProfile(client, "valid-token-profile-read")

	// Test 2: Valid token with profile:write scope
	fmt.Println("\n2. UpdateProfile with valid token (profile:write scope)")
	testUpdateProfile(client, "valid-token-profile-write")

	// Test 3: Admin token with all scopes
	fmt.Println("\n3. ListUsers with admin token (all scopes)")
	testListUsers(client, "valid-token-admin")

	// Test 4: Invalid token
	fmt.Println("\n4. GetProfile with invalid token")
	testGetProfile(client, "invalid-token")

	// Test 5: Missing token
	fmt.Println("\n5. GetProfile without token")
	testGetProfileNoAuth(client)

	// Test 6: Insufficient scopes
	fmt.Println("\n6. UpdateProfile with read-only token (insufficient scopes)")
	testUpdateProfile(client, "valid-token-profile-read")

	// Test 7: Expired token
	fmt.Println("\n7. GetProfile with expired token")
	testGetProfile(client, "expired-token")
}

func testGetProfile(client pb.UserServiceClient, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add authorization header
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	resp, err := client.GetProfile(ctx, &pb.GetProfileRequest{
		UserId: "123",
	})

	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
		return
	}

	fmt.Printf("   ✅ Success: user_id=%s, username=%s, email=%s, scopes=%v\n",
		resp.UserId, resp.Username, resp.Email, resp.Scopes)
}

func testGetProfileNoAuth(client pb.UserServiceClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// No authorization header
	resp, err := client.GetProfile(ctx, &pb.GetProfileRequest{
		UserId: "123",
	})

	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
		return
	}

	fmt.Printf("   ✅ Success: user_id=%s, username=%s, email=%s\n",
		resp.UserId, resp.Username, resp.Email)
}

func testUpdateProfile(client pb.UserServiceClient, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add authorization header
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	resp, err := client.UpdateProfile(ctx, &pb.UpdateProfileRequest{
		UserId:   "123",
		Username: "newname",
		Email:    "new@example.com",
	})

	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
		return
	}

	fmt.Printf("   ✅ Success: %s\n", resp.Message)
}

func testListUsers(client pb.UserServiceClient, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add authorization header
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	resp, err := client.ListUsers(ctx, &pb.ListUsersRequest{
		Page:     1,
		PageSize: 10,
	})

	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
		return
	}

	fmt.Printf("   ✅ Success: found %d users\n", resp.Total)
	for _, user := range resp.Users {
		fmt.Printf("      - %s (%s)\n", user.Username, user.Email)
	}
}
