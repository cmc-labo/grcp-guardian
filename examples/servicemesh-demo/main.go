package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grpc-guardian/grpc-guardian/middleware"
	"github.com/grpc-guardian/grpc-guardian/pkg/servicemesh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Example gRPC service
type greeterServer struct{}

func (s *greeterServer) SayHello(ctx context.Context, req *HelloRequest) (*HelloResponse, error) {
	// Simulate some processing
	time.Sleep(50 * time.Millisecond)

	return &HelloResponse{
		Message: fmt.Sprintf("Hello, %s!", req.Name),
	}, nil
}

// HelloRequest and HelloResponse are example message types
type HelloRequest struct {
	Name string
}

type HelloResponse struct {
	Message string
}

func main() {
	// Determine which service mesh to use from environment
	meshProvider := os.Getenv("MESH_PROVIDER")
	if meshProvider == "" {
		meshProvider = "istio" // Default to Istio
	}

	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "greeter-service"
	}

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	log.Printf("Starting gRPC Guardian Service Mesh Demo")
	log.Printf("Service: %s, Namespace: %s, Mesh: %s", serviceName, namespace, meshProvider)

	// Create service mesh middleware
	var meshMiddleware *middleware.ServiceMeshMiddleware
	var err error

	switch meshProvider {
	case "istio":
		meshMiddleware, err = createIstioMiddleware(serviceName, namespace)
	case "linkerd":
		meshMiddleware, err = createLinkerdMiddleware(serviceName, namespace)
	default:
		log.Fatalf("Unknown mesh provider: %s. Use 'istio' or 'linkerd'", meshProvider)
	}

	if err != nil {
		log.Fatalf("Failed to create mesh middleware: %v", err)
	}

	// Create gRPC server with mesh middleware
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.Logging(),
			meshMiddleware.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			meshMiddleware.StreamServerInterceptor(),
		),
	)

	// Register services
	// pb.RegisterGreeterServer(server, &greeterServer{})

	// Register health check
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// Enable reflection for debugging
	reflection.Register(server)

	// Start server
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down gracefully...")
		server.GracefulStop()
	}()

	log.Printf("Server listening on :%s", port)
	log.Println("Service mesh integration enabled!")
	log.Println("\nExample requests:")
	log.Println("  Istio: Automatic header propagation (x-request-id, x-b3-traceid, etc.)")
	log.Println("  Linkerd: Automatic header propagation (l5d-ctx-trace, l5d-ctx-traceid, etc.)")
	log.Println("\nFeatures enabled:")
	log.Println("  ✓ Distributed tracing header propagation")
	log.Println("  ✓ Service mesh metadata extraction")
	log.Println("  ✓ Request logging with mesh context")
	log.Println("  ✓ mTLS validation (if enabled)")

	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// createIstioMiddleware creates Istio service mesh middleware
func createIstioMiddleware(serviceName, namespace string) (*middleware.ServiceMeshMiddleware, error) {
	enableMTLS := os.Getenv("ENABLE_MTLS") == "true"

	config := &servicemesh.Config{
		Provider:               servicemesh.ProviderIstio,
		ServiceName:            serviceName,
		Namespace:              namespace,
		EnableMTLS:             enableMTLS,
		EnableTrafficSplitting: false,
		CustomHeaders: []string{
			"x-custom-header",
			"x-user-id",
		},
	}

	opts := []middleware.ServiceMeshOption{
		middleware.WithHeaderPropagation(),
		middleware.WithMetadataLogging(),
		middleware.WithMetadataCallback(func(metadata *servicemesh.MeshMetadata) {
			log.Printf("[Istio] Request metadata:")
			log.Printf("  RequestID: %s", metadata.RequestID)
			log.Printf("  TraceID: %s", metadata.TraceID)
			log.Printf("  Source: %s/%s", metadata.SourceNamespace, metadata.SourceWorkload)
			if len(metadata.CustomLabels) > 0 {
				log.Printf("  Custom labels: %v", metadata.CustomLabels)
			}
		}),
		middleware.WithErrorCallback(func(err error) {
			log.Printf("[Istio] Error: %v", err)
		}),
	}

	if enableMTLS {
		opts = append(opts, middleware.WithMTLSValidation())
		log.Println("mTLS validation enabled")
	}

	return middleware.Istio(config, opts...)
}

// createLinkerdMiddleware creates Linkerd service mesh middleware
func createLinkerdMiddleware(serviceName, namespace string) (*middleware.ServiceMeshMiddleware, error) {
	enableMTLS := os.Getenv("ENABLE_MTLS") == "true"

	config := &servicemesh.Config{
		Provider:               servicemesh.ProviderLinkerd,
		ServiceName:            serviceName,
		Namespace:              namespace,
		EnableMTLS:             enableMTLS,
		EnableTrafficSplitting: false,
		CustomHeaders: []string{
			"l5d-dtab",
		},
	}

	opts := []middleware.ServiceMeshOption{
		middleware.WithHeaderPropagation(),
		middleware.WithMetadataLogging(),
		middleware.WithMetadataCallback(func(metadata *servicemesh.MeshMetadata) {
			log.Printf("[Linkerd] Request metadata:")
			log.Printf("  RequestID: %s", metadata.RequestID)
			log.Printf("  TraceID: %s", metadata.TraceID)
			if dtab, ok := metadata.CustomLabels["dtab"]; ok {
				log.Printf("  Dtab: %s", dtab)
			}
		}),
		middleware.WithErrorCallback(func(err error) {
			log.Printf("[Linkerd] Error: %v", err)
		}),
	}

	if enableMTLS {
		opts = append(opts, middleware.WithMTLSValidation())
		log.Println("mTLS validation enabled")
	}

	return middleware.Linkerd(config, opts...)
}

// Example client code
func exampleClient() {
	// Determine mesh provider
	meshProvider := os.Getenv("MESH_PROVIDER")
	if meshProvider == "" {
		meshProvider = "istio"
	}

	// Create client middleware
	var meshMiddleware *middleware.ServiceMeshMiddleware
	var err error

	switch meshProvider {
	case "istio":
		meshMiddleware, err = middleware.IstioSimple("client-service", "default")
	case "linkerd":
		meshMiddleware, err = middleware.LinkerdSimple("client-service", "default")
	}

	if err != nil {
		log.Fatalf("Failed to create client middleware: %v", err)
	}

	// Create gRPC client connection with mesh middleware
	conn, err := grpc.Dial(
		"greeter-service:50051",
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(meshMiddleware.UnaryClientInterceptor()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Use the connection
	// client := pb.NewGreeterClient(conn)
	// resp, err := client.SayHello(context.Background(), &pb.HelloRequest{Name: "World"})

	log.Println("Client configured with service mesh integration")
	log.Println("Headers will be automatically propagated across service calls")
}

// Kubernetes deployment example
const kubernetesYAML = `
# Example Kubernetes deployment for Istio
apiVersion: v1
kind: Service
metadata:
  name: greeter-service
  namespace: default
spec:
  ports:
  - port: 50051
    name: grpc
  selector:
    app: greeter
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: greeter
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: greeter
  template:
    metadata:
      labels:
        app: greeter
        version: v1
      annotations:
        # Istio sidecar injection
        sidecar.istio.io/inject: "true"
    spec:
      containers:
      - name: greeter
        image: your-registry/greeter:latest
        ports:
        - containerPort: 50051
        env:
        - name: MESH_PROVIDER
          value: "istio"
        - name: SERVICE_NAME
          value: "greeter-service"
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: ENABLE_MTLS
          value: "true"
---
# Istio VirtualService for traffic splitting
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: greeter
spec:
  hosts:
  - greeter-service
  http:
  - match:
    - headers:
        x-canary:
          exact: "true"
    route:
    - destination:
        host: greeter-service
        subset: v2
      weight: 100
  - route:
    - destination:
        host: greeter-service
        subset: v1
      weight: 90
    - destination:
        host: greeter-service
        subset: v2
      weight: 10
---
# Istio DestinationRule
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: greeter
spec:
  host: greeter-service
  trafficPolicy:
    tls:
      mode: ISTIO_MUTUAL
  subsets:
  - name: v1
    labels:
      version: v1
  - name: v2
    labels:
      version: v2
`

func init() {
	// Print example deployment on startup if requested
	if os.Getenv("PRINT_DEPLOYMENT") == "true" {
		fmt.Println(kubernetesYAML)
		os.Exit(0)
	}
}
