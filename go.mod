module github.com/grpc-guardian/grpc-guardian

go 1.21

require (
	google.golang.org/grpc v1.59.0
	google.golang.org/protobuf v1.31.0
	golang.org/x/time v0.5.0
	github.com/golang-jwt/jwt/v5 v5.2.0
	go.uber.org/zap v1.26.0
	go.opentelemetry.io/otel v1.21.0
	go.opentelemetry.io/otel/trace v1.21.0
	go.opentelemetry.io/otel/sdk v1.21.0
	go.opentelemetry.io/otel/exporters/jaeger v1.17.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.46.1
	github.com/prometheus/client_golang v1.17.0
	github.com/prometheus/client_model v0.5.0
	github.com/stretchr/testify v1.8.4
)
