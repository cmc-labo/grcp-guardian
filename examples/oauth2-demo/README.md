# OAuth2 Authentication Demo

This example demonstrates OAuth 2.0 token introspection authentication (RFC 7662) with scope-based authorization in gRPC Guardian.

## Features

- OAuth 2.0 token introspection (RFC 7662)
- Scope-based authorization
- Context enrichment with user claims
- Mock OAuth2 introspection server for testing
- Per-method scope requirements

## Prerequisites

```bash
# Install grpcurl for testing
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Install protoc and Go plugins (if you want to regenerate protos)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

## Running the Demo

1. Start the server:

```bash
cd examples/oauth2-demo
go run main.go
```

This will start:
- gRPC server on `:50051`
- Mock OAuth2 introspection server on `:8081`

## Testing

### 1. Valid Token with `profile:read` Scope

```bash
grpcurl -plaintext \
  -H "Authorization: Bearer valid-token-profile-read" \
  -d '{"user_id": "123"}' \
  localhost:50051 user.UserService/GetProfile
```

Expected response:
```json
{
  "userId": "123",
  "username": "alice",
  "email": "alice@example.com",
  "scopes": ["profile:read"]
}
```

### 2. Valid Token with `profile:write` Scope

```bash
grpcurl -plaintext \
  -H "Authorization: Bearer valid-token-profile-write" \
  -d '{"user_id": "123", "username": "newname", "email": "new@example.com"}' \
  localhost:50051 user.UserService/UpdateProfile
```

Expected response:
```json
{
  "success": true,
  "message": "Profile updated successfully"
}
```

### 3. Admin Token with All Scopes

```bash
grpcurl -plaintext \
  -H "Authorization: Bearer valid-token-admin" \
  -d '{"page": 1, "page_size": 10}' \
  localhost:50051 user.UserService/ListUsers
```

Expected response:
```json
{
  "users": [
    {"userId": "1", "username": "alice", "email": "alice@example.com"},
    {"userId": "2", "username": "bob", "email": "bob@example.com"},
    {"userId": "3", "username": "charlie", "email": "charlie@example.com"}
  ],
  "total": 3
}
```

### 4. Invalid Token

```bash
grpcurl -plaintext \
  -H "Authorization: Bearer invalid-token" \
  -d '{"user_id": "123"}' \
  localhost:50051 user.UserService/GetProfile
```

Expected error:
```
ERROR:
  Code: Unauthenticated
  Message: token is not active
```

### 5. Missing Token

```bash
grpcurl -plaintext \
  -d '{"user_id": "123"}' \
  localhost:50051 user.UserService/GetProfile
```

Expected error:
```
ERROR:
  Code: Unauthenticated
  Message: missing authorization header
```

### 6. Insufficient Scopes

Try to update profile with read-only token:

```bash
grpcurl -plaintext \
  -H "Authorization: Bearer valid-token-profile-read" \
  -d '{"user_id": "123", "username": "newname"}' \
  localhost:50051 user.UserService/UpdateProfile
```

Expected error:
```
ERROR:
  Code: PermissionDenied
  Message: insufficient permissions: requires one of scopes [profile:write], user has [profile:read]
```

## Mock OAuth2 Tokens

The demo includes a mock OAuth2 introspection server with the following test tokens:

| Token | Scopes | Username | Status |
|-------|--------|----------|--------|
| `valid-token-profile-read` | `profile:read` | alice | Active |
| `valid-token-profile-write` | `profile:write profile:read` | bob | Active |
| `valid-token-admin` | `profile:read profile:write users:read users:write` | admin | Active |
| `expired-token` | - | - | Inactive |
| `invalid-token` | - | - | Inactive |

## OAuth2 Configuration

```go
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "http://localhost:8081/introspect",
    ClientID:         "grpc-guardian-service",
    ClientSecret:     "service-secret",
    Timeout:          5 * time.Second,
}

// Use in middleware chain
chain := guardian.NewChain(
    middleware.Logging(),
    middleware.Auth(middleware.OAuth2Validator(oauth2Config)),
)
```

## Scope-Based Authorization

```go
// Require specific scopes for methods
middleware.RequireScope("profile:read")      // Read user profile
middleware.RequireScope("profile:write")     // Update user profile
middleware.RequireScope("users:read")        // List all users
middleware.RequireScope("users:write")       // Create/delete users
```

## Context Values

After successful OAuth2 authentication, the following values are added to the context:

- `user_id` (string): Subject (sub) from token
- `username` (string): Human-readable username
- `client_id` (string): OAuth2 client identifier
- `scopes` ([]string): Array of granted scopes
- `oauth2_introspection` (oauth2IntrospectionResponse): Full introspection response

### Accessing Context Values

```go
func (s *server) MyMethod(ctx context.Context, req *MyRequest) (*MyResponse, error) {
    // Get user information
    userID, _ := ctx.Value("user_id").(string)
    username, _ := ctx.Value("username").(string)
    clientID, _ := ctx.Value("client_id").(string)

    // Get scopes using helper function
    scopes, ok := middleware.GetScopes(ctx)
    if !ok {
        return nil, status.Error(codes.Internal, "scopes not found")
    }

    // Get client ID using helper function
    clientID, ok := middleware.GetClientID(ctx)

    // Your business logic here
    log.Printf("Request from user=%s, client=%s, scopes=%v", username, clientID, scopes)

    return &MyResponse{}, nil
}
```

## Production OAuth2 Providers

### Auth0

```go
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "https://YOUR_DOMAIN.auth0.com/oauth/token/introspect",
    ClientID:         "YOUR_CLIENT_ID",
    ClientSecret:     "YOUR_CLIENT_SECRET",
}
```

### Keycloak

```go
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "https://keycloak.example.com/auth/realms/YOUR_REALM/protocol/openid-connect/token/introspect",
    ClientID:         "YOUR_CLIENT_ID",
    ClientSecret:     "YOUR_CLIENT_SECRET",
}
```

### Okta

```go
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "https://YOUR_DOMAIN.okta.com/oauth2/default/v1/introspect",
    ClientID:         "YOUR_CLIENT_ID",
    ClientSecret:     "YOUR_CLIENT_SECRET",
}
```

### Google Cloud Identity Platform

```go
oauth2Config := middleware.OAuth2Config{
    IntrospectionURL: "https://oauth2.googleapis.com/tokeninfo",
    ClientID:         "YOUR_CLIENT_ID.apps.googleusercontent.com",
    ClientSecret:     "YOUR_CLIENT_SECRET",
}
```

## Security Best Practices

1. **Use HTTPS in Production**: Always use TLS for the introspection endpoint
2. **Secure Credentials**: Store client secrets in environment variables or secret managers
3. **Set Appropriate Timeouts**: Configure reasonable timeouts for introspection requests
4. **Validate Scopes**: Always validate required scopes for sensitive operations
5. **Cache Tokens**: Consider implementing token caching to reduce introspection calls
6. **Monitor Failed Attempts**: Log and monitor authentication failures

## Performance Considerations

- Each request requires an HTTP call to the introspection endpoint
- Consider implementing a caching layer for valid tokens
- Use connection pooling for the HTTP client
- Set appropriate timeouts to prevent slow responses

## Troubleshooting

### "missing authorization header" Error

Make sure you're sending the Authorization header:
```bash
-H "Authorization: Bearer YOUR_TOKEN"
```

### "introspection request failed" Error

- Check that the OAuth2 server is running and accessible
- Verify the introspection URL is correct
- Check network connectivity

### "token is not active" Error

- Token may be expired
- Token may be invalid or revoked
- Check token format (should be a valid access token)

### "insufficient permissions" Error

- User doesn't have required scope
- Check scope requirements match your OAuth2 provider's scope format

## References

- [RFC 7662: OAuth 2.0 Token Introspection](https://datatracker.ietf.org/doc/html/rfc7662)
- [OAuth 2.0 Scopes](https://www.oauth.com/oauth2-servers/scope/)
- [gRPC Authentication Guide](https://grpc.io/docs/guides/auth/)
