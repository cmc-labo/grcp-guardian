package middleware

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LoggingConfig holds configuration for logging middleware
type LoggingConfig struct {
	Logger          *zap.Logger
	Level           zapcore.Level
	LogRequestBody  bool
	LogResponseBody bool
	ExtraFields     map[string]interface{}
}

// LoggingOption is a functional option for logging configuration
type LoggingOption func(*LoggingConfig)

// WithLogger sets a custom zap logger
func WithLogger(logger *zap.Logger) LoggingOption {
	return func(c *LoggingConfig) {
		c.Logger = logger
	}
}

// WithLevel sets the logging level
func WithLevel(level zapcore.Level) LoggingOption {
	return func(c *LoggingConfig) {
		c.Level = level
	}
}

// WithRequestBody enables request body logging
func WithRequestBody() LoggingOption {
	return func(c *LoggingConfig) {
		c.LogRequestBody = true
	}
}

// WithResponseBody enables response body logging
func WithResponseBody() LoggingOption {
	return func(c *LoggingConfig) {
		c.LogResponseBody = true
	}
}

// WithExtraFields adds extra fields to all log entries
func WithExtraFields(fields map[string]interface{}) LoggingOption {
	return func(c *LoggingConfig) {
		c.ExtraFields = fields
	}
}

// Logging creates a logging middleware with the provided options
func Logging(opts ...LoggingOption) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Default configuration
	config := &LoggingConfig{
		Logger: zap.NewExample(),
		Level:  zapcore.InfoLevel,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Build base fields
		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Time("start_time", start),
		}

		// Add extra fields
		for k, v := range config.ExtraFields {
			fields = append(fields, zap.Any(k, v))
		}

		// Add request body if enabled
		if config.LogRequestBody {
			fields = append(fields, zap.Any("request", req))
		}

		// Add user context if available
		if userID, ok := GetUserID(ctx); ok {
			fields = append(fields, zap.String("user_id", userID))
		}

		config.Logger.Info("gRPC request started", fields...)

		// Call handler
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Build response fields
		responseFields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
			zap.Int64("duration_ms", duration.Milliseconds()),
		}

		// Add extra fields
		for k, v := range config.ExtraFields {
			responseFields = append(responseFields, zap.Any(k, v))
		}

		// Add user context if available
		if userID, ok := GetUserID(ctx); ok {
			responseFields = append(responseFields, zap.String("user_id", userID))
		}

		// Add response body if enabled and no error
		if config.LogResponseBody && err == nil {
			responseFields = append(responseFields, zap.Any("response", resp))
		}

		// Log based on error status
		if err != nil {
			st := status.Convert(err)
			responseFields = append(responseFields,
				zap.String("grpc_code", st.Code().String()),
				zap.String("error", st.Message()),
			)

			// Log level based on error code
			switch st.Code() {
			case codes.Internal, codes.Unknown, codes.DataLoss:
				config.Logger.Error("gRPC request failed", responseFields...)
			case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
				codes.PermissionDenied, codes.Unauthenticated:
				config.Logger.Warn("gRPC request rejected", responseFields...)
			default:
				config.Logger.Info("gRPC request completed with error", responseFields...)
			}
		} else {
			responseFields = append(responseFields, zap.String("grpc_code", codes.OK.String()))
			config.Logger.Info("gRPC request completed", responseFields...)
		}

		return resp, err
	}
}

// AccessLog creates a simple access log middleware (Apache/Nginx style)
func AccessLog() func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	logger, _ := zap.NewProduction()

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Call handler
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Determine status code
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		// Log in access log format
		logger.Info("access",
			zap.String("method", info.FullMethod),
			zap.String("code", code.String()),
			zap.Duration("duration", duration),
			zap.Time("time", start),
		)

		return resp, err
	}
}

// PerformanceLog logs performance metrics for slow requests
func PerformanceLog(threshold time.Duration) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	logger, _ := zap.NewProduction()

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Call handler
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Log if request exceeded threshold
		if duration > threshold {
			logger.Warn("slow request detected",
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration),
				zap.Duration("threshold", threshold),
			)
		}

		return resp, err
	}
}
