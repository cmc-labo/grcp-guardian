package middleware

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Circuit Breaker States
const (
	StateClosed   State = iota // Normal operation, requests pass through
	StateOpen                  // Circuit is open, requests fail immediately
	StateHalfOpen              // Testing if service has recovered
)

// State represents the current state of the circuit breaker
type State int

func (s State) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrTooManyRequests is returned when too many requests are made in half-open state
	ErrTooManyRequests = errors.New("too many requests")
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu sync.RWMutex

	// Configuration
	maxRequests        uint32        // Max requests allowed in half-open state
	interval           time.Duration // Time window for counting failures
	timeout            time.Duration // Time to wait in open state before trying half-open
	failureThreshold   float64       // Percentage of failures to trigger open state (0.0-1.0)
	successThreshold   uint32        // Consecutive successes needed to close from half-open

	// State tracking
	state            State
	generation       uint64
	stateChangedAt   time.Time

	// Counters
	counts           Counts
	halfOpenRequests uint32

	// Callbacks
	onStateChange func(from, to State)
	isFailure     func(err error) bool
}

// Counts holds the statistics for the circuit breaker
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// CircuitBreakerOption configures a CircuitBreaker
type CircuitBreakerOption func(*CircuitBreaker)

// WithMaxRequests sets the maximum number of requests allowed in half-open state
func WithMaxRequests(n uint32) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.maxRequests = n
	}
}

// WithInterval sets the time window for counting failures
func WithInterval(d time.Duration) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.interval = d
	}
}

// WithTimeout sets the time to wait in open state before half-open
func WithTimeout(d time.Duration) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.timeout = d
	}
}

// WithFailureThreshold sets the failure percentage threshold
func WithFailureThreshold(threshold float64) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		if threshold > 0 && threshold <= 1.0 {
			cb.failureThreshold = threshold
		}
	}
}

// WithSuccessThreshold sets the consecutive successes needed to close
func WithSuccessThreshold(n uint32) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.successThreshold = n
	}
}

// WithOnStateChange sets a callback for state changes
func WithOnStateChange(fn func(from, to State)) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.onStateChange = fn
	}
}

// WithIsFailure sets a custom function to determine if an error is a failure
func WithIsFailure(fn func(err error) bool) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.isFailure = fn
	}
}

// NewCircuitBreaker creates a new circuit breaker with default settings
func NewCircuitBreaker(opts ...CircuitBreakerOption) *CircuitBreaker {
	cb := &CircuitBreaker{
		maxRequests:      1,
		interval:         60 * time.Second,
		timeout:          60 * time.Second,
		failureThreshold: 0.6, // 60% failure rate
		successThreshold: 1,
		state:            StateClosed,
		stateChangedAt:   time.Now(),
		isFailure:        defaultIsFailure,
	}

	// Apply options
	for _, opt := range opts {
		opt(cb)
	}

	return cb
}

// defaultIsFailure determines if an error should be counted as a failure
func defaultIsFailure(err error) bool {
	if err == nil {
		return false
	}

	// Consider these gRPC codes as failures that should trip the circuit
	st, ok := status.FromError(err)
	if !ok {
		return true
	}

	switch st.Code() {
	case codes.Internal,
		codes.Unavailable,
		codes.DataLoss,
		codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

// CircuitBreaker returns a middleware that implements the circuit breaker pattern
func CircuitBreakerMiddleware(opts ...CircuitBreakerOption) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	cb := NewCircuitBreaker(opts...)

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Check if request is allowed
		generation, err := cb.beforeRequest()
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "circuit breaker: %v", err)
		}

		// Execute the request
		resp, err := handler(ctx, req)

		// Record the result
		cb.afterRequest(generation, err)

		return resp, err
	}
}

// beforeRequest checks if the request is allowed based on circuit breaker state
func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		return generation, ErrCircuitOpen
	}

	if state == StateHalfOpen {
		if cb.halfOpenRequests >= cb.maxRequests {
			return generation, ErrTooManyRequests
		}
		cb.halfOpenRequests++
	}

	cb.counts.Requests++
	return generation, nil
}

// afterRequest records the result of a request
func (cb *CircuitBreaker) afterRequest(generation uint64, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, currentGeneration := cb.currentState(now)

	// Ignore if generation doesn't match (state changed during request)
	if generation != currentGeneration {
		return
	}

	if cb.isFailure(err) {
		cb.counts.TotalFailures++
		cb.counts.ConsecutiveFailures++
		cb.counts.ConsecutiveSuccesses = 0

		if state == StateHalfOpen {
			// Failed during half-open, go back to open
			cb.setState(StateOpen, now)
		} else if state == StateClosed {
			// Check if we should open the circuit
			if cb.shouldOpen() {
				cb.setState(StateOpen, now)
			}
		}
	} else {
		cb.counts.TotalSuccesses++
		cb.counts.ConsecutiveSuccesses++
		cb.counts.ConsecutiveFailures = 0

		if state == StateHalfOpen {
			// Check if we should close the circuit
			if cb.counts.ConsecutiveSuccesses >= cb.successThreshold {
				cb.setState(StateClosed, now)
			}
		}
	}
}

// currentState returns the current state and generation
func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		// Check if interval has expired, reset counts
		if cb.interval > 0 && now.Sub(cb.stateChangedAt) > cb.interval {
			cb.resetCounts()
		}
	case StateOpen:
		// Check if timeout has expired, transition to half-open
		if now.Sub(cb.stateChangedAt) >= cb.timeout {
			cb.setState(StateHalfOpen, now)
		}
	}

	return cb.state, cb.generation
}

// shouldOpen determines if the circuit should open based on failure threshold
func (cb *CircuitBreaker) shouldOpen() bool {
	if cb.counts.Requests < 10 {
		// Need minimum number of requests before opening
		return false
	}

	failureRate := float64(cb.counts.TotalFailures) / float64(cb.counts.Requests)
	return failureRate >= cb.failureThreshold
}

// setState changes the circuit breaker state
func (cb *CircuitBreaker) setState(newState State, now time.Time) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.stateChangedAt = now
	cb.generation++

	// Reset counters on state change
	cb.resetCounts()

	if newState == StateHalfOpen {
		cb.halfOpenRequests = 0
	}

	// Call state change callback
	if cb.onStateChange != nil {
		cb.onStateChange(oldState, newState)
	}
}

// resetCounts resets all counters
func (cb *CircuitBreaker) resetCounts() {
	cb.counts = Counts{}
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.state
}

// Counts returns the current circuit breaker counts
func (cb *CircuitBreaker) GetCounts() Counts {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.counts
}

// Stats returns detailed statistics about the circuit breaker
type Stats struct {
	State                State
	Counts               Counts
	StateChangedAt       time.Time
	Generation           uint64
}

// GetStats returns current statistics
func (cb *CircuitBreaker) GetStats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return Stats{
		State:          cb.state,
		Counts:         cb.counts,
		StateChangedAt: cb.stateChangedAt,
		Generation:     cb.generation,
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateClosed, time.Now())
}
