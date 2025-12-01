package middleware

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RateLimiter interface for rate limiting implementations
type RateLimiter interface {
	Allow() bool
	Wait(ctx context.Context) error
}

// RateLimit creates a global rate limiting middleware using token bucket algorithm
// rate: tokens per second
// burst: maximum burst size
func RateLimit(ratePerSec int, burst int) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	limiter := rate.NewLimiter(rate.Limit(ratePerSec), burst)

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !limiter.Allow() {
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
		}

		return handler(ctx, req)
	}
}

// RateLimitWithWait creates a rate limiting middleware that waits instead of rejecting
func RateLimitWithWait(ratePerSec int, burst int) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	limiter := rate.NewLimiter(rate.Limit(ratePerSec), burst)

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := limiter.Wait(ctx); err != nil {
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit wait failed: %v", err)
		}

		return handler(ctx, req)
	}
}

// PerClientRateLimiter manages rate limiters for individual clients
type PerClientRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewPerClientRateLimiter creates a new per-client rate limiter
func NewPerClientRateLimiter(ratePerSec int, burst int) *PerClientRateLimiter {
	return &PerClientRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(ratePerSec),
		burst:    burst,
	}
}

// GetLimiter returns a rate limiter for the given client
func (p *PerClientRateLimiter) GetLimiter(clientID string) *rate.Limiter {
	p.mu.RLock()
	limiter, exists := p.limiters[clientID]
	p.mu.RUnlock()

	if exists {
		return limiter
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := p.limiters[clientID]; exists {
		return limiter
	}

	// Create new limiter
	limiter = rate.NewLimiter(p.rate, p.burst)
	p.limiters[clientID] = limiter

	return limiter
}

// RateLimitPerClient creates a per-client rate limiting middleware
// clientIDExtractor: function to extract client ID from context
func RateLimitPerClient(ratePerSec int, burst int, clientIDExtractor func(context.Context) string) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	perClientLimiter := NewPerClientRateLimiter(ratePerSec, burst)

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		clientID := clientIDExtractor(ctx)
		if clientID == "" {
			clientID = "unknown"
		}

		limiter := perClientLimiter.GetLimiter(clientID)
		if !limiter.Allow() {
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded for client: %s", clientID)
		}

		return handler(ctx, req)
	}
}

// ExtractClientIP extracts client IP from gRPC metadata
func ExtractClientIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "unknown"
	}

	// Try X-Forwarded-For first (for proxied requests)
	if xff := md.Get("x-forwarded-for"); len(xff) > 0 {
		return xff[0]
	}

	// Try X-Real-IP
	if xri := md.Get("x-real-ip"); len(xri) > 0 {
		return xri[0]
	}

	return "unknown"
}

// ExtractUserID extracts user ID from context (set by auth middleware)
func ExtractUserIDForRateLimit(ctx context.Context) string {
	if userID, ok := GetUserID(ctx); ok {
		return userID
	}
	return "anonymous"
}

// PerMethodRateLimiter manages different rate limits for different methods
type PerMethodRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	defaults *rate.Limiter
}

// NewPerMethodRateLimiter creates a per-method rate limiter
func NewPerMethodRateLimiter(defaultRate int, defaultBurst int) *PerMethodRateLimiter {
	return &PerMethodRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		defaults: rate.NewLimiter(rate.Limit(defaultRate), defaultBurst),
	}
}

// SetMethodLimit sets a specific rate limit for a method
func (p *PerMethodRateLimiter) SetMethodLimit(method string, ratePerSec int, burst int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.limiters[method] = rate.NewLimiter(rate.Limit(ratePerSec), burst)
}

// GetLimiter returns the rate limiter for a specific method
func (p *PerMethodRateLimiter) GetLimiter(method string) *rate.Limiter {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if limiter, exists := p.limiters[method]; exists {
		return limiter
	}

	return p.defaults
}

// RateLimitPerMethod creates a per-method rate limiting middleware
func RateLimitPerMethod(defaultRate int, defaultBurst int, methodLimits map[string]struct{ Rate, Burst int }) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	perMethodLimiter := NewPerMethodRateLimiter(defaultRate, defaultBurst)

	// Set method-specific limits
	for method, limits := range methodLimits {
		perMethodLimiter.SetMethodLimit(method, limits.Rate, limits.Burst)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		limiter := perMethodLimiter.GetLimiter(info.FullMethod)

		if !limiter.Allow() {
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded for method: %s", info.FullMethod)
		}

		return handler(ctx, req)
	}
}

// AdaptiveRateLimiter adjusts rate limits based on system load
type AdaptiveRateLimiter struct {
	limiter     *rate.Limiter
	mu          sync.RWMutex
	baseRate    rate.Limit
	currentRate rate.Limit
	burst       int
}

// NewAdaptiveRateLimiter creates an adaptive rate limiter
func NewAdaptiveRateLimiter(baseRate int, burst int) *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		limiter:     rate.NewLimiter(rate.Limit(baseRate), burst),
		baseRate:    rate.Limit(baseRate),
		currentRate: rate.Limit(baseRate),
		burst:       burst,
	}
}

// AdjustRate adjusts the rate limit based on load factor (0.0 - 1.0)
// loadFactor < 0.5: increase rate
// loadFactor > 0.8: decrease rate
func (a *AdaptiveRateLimiter) AdjustRate(loadFactor float64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var newRate rate.Limit

	if loadFactor < 0.5 {
		// Low load - increase rate by 20%
		newRate = a.currentRate * 1.2
		if newRate > a.baseRate*2 {
			newRate = a.baseRate * 2 // Cap at 2x base rate
		}
	} else if loadFactor > 0.8 {
		// High load - decrease rate by 20%
		newRate = a.currentRate * 0.8
		if newRate < a.baseRate*0.5 {
			newRate = a.baseRate * 0.5 // Floor at 0.5x base rate
		}
	} else {
		// Moderate load - gradually return to base rate
		if a.currentRate > a.baseRate {
			newRate = a.currentRate * 0.95
		} else if a.currentRate < a.baseRate {
			newRate = a.currentRate * 1.05
		} else {
			return // Already at base rate
		}
	}

	a.currentRate = newRate
	a.limiter.SetLimit(newRate)
}

// Allow checks if a request is allowed
func (a *AdaptiveRateLimiter) Allow() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.limiter.Allow()
}
