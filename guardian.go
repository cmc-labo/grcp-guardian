// Package guardian provides a high-performance, modular middleware library for gRPC
package guardian

import (
	"context"

	"google.golang.org/grpc"
)

// Middleware defines the interface for gRPC middleware
// It wraps a UnaryHandler and returns a new UnaryHandler
type Middleware func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)

// StreamMiddleware defines the interface for streaming gRPC middleware
type StreamMiddleware func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error

// Chain represents a chain of middleware
type Chain struct {
	middlewares       []Middleware
	streamMiddlewares []StreamMiddleware
}

// NewChain creates a new middleware chain
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{
		middlewares: middlewares,
	}
}

// Append adds middleware to the end of the chain
func (c *Chain) Append(middlewares ...Middleware) *Chain {
	c.middlewares = append(c.middlewares, middlewares...)
	return c
}

// Prepend adds middleware to the beginning of the chain
func (c *Chain) Prepend(middlewares ...Middleware) *Chain {
	c.middlewares = append(middlewares, c.middlewares...)
	return c
}

// UnaryInterceptor returns a gRPC UnaryServerInterceptor that executes the middleware chain
func (c *Chain) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Build the chain of handlers
		currentHandler := handler

		// Apply middleware in reverse order so they execute in the correct order
		for i := len(c.middlewares) - 1; i >= 0; i-- {
			middleware := c.middlewares[i]
			next := currentHandler

			// Wrap the handler with the middleware
			currentHandler = func(ctx context.Context, req interface{}) (interface{}, error) {
				return middleware(ctx, req, info, next)
			}
		}

		// Execute the chain
		return currentHandler(ctx, req)
	}
}

// StreamInterceptor returns a gRPC StreamServerInterceptor that executes the middleware chain
func (c *Chain) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Build the chain of handlers
		currentHandler := handler

		// Apply middleware in reverse order
		for i := len(c.streamMiddlewares) - 1; i >= 0; i-- {
			middleware := c.streamMiddlewares[i]
			next := currentHandler

			// Wrap the handler with the middleware
			currentHandler = func(srv interface{}, ss grpc.ServerStream) error {
				return middleware(srv, ss, info, next)
			}
		}

		// Execute the chain
		return currentHandler(srv, ss)
	}
}

// ServerOption returns a gRPC ServerOption with both unary and stream interceptors
func (c *Chain) ServerOption() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(c.UnaryInterceptor()),
		grpc.StreamInterceptor(c.StreamInterceptor()),
	}
}

// ChainUnaryServer creates a single interceptor from multiple unary server interceptors
func ChainUnaryServer(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		currentHandler := handler

		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := currentHandler

			currentHandler = func(ctx context.Context, req interface{}) (interface{}, error) {
				return interceptor(ctx, req, info, next)
			}
		}

		return currentHandler(ctx, req)
	}
}

// ChainStreamServer creates a single interceptor from multiple stream server interceptors
func ChainStreamServer(interceptors ...grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		currentHandler := handler

		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := currentHandler

			currentHandler = func(srv interface{}, ss grpc.ServerStream) error {
				return interceptor(srv, ss, info, next)
			}
		}

		return currentHandler(srv, ss)
	}
}
