package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc"
)

// KeyGenerator generates cache keys from gRPC requests
type KeyGenerator interface {
	// GenerateKey creates a cache key from the request
	GenerateKey(method string, req interface{}) (string, error)
}

// DefaultKeyGenerator is the default key generation strategy
type DefaultKeyGenerator struct{}

// NewDefaultKeyGenerator creates a new default key generator
func NewDefaultKeyGenerator() *DefaultKeyGenerator {
	return &DefaultKeyGenerator{}
}

// GenerateKey generates a cache key based on method name and request hash
func (g *DefaultKeyGenerator) GenerateKey(method string, req interface{}) (string, error) {
	// Serialize request to JSON
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create hash of request
	hash := sha256.Sum256(reqBytes)
	hashStr := hex.EncodeToString(hash[:])

	// Combine method and hash
	key := fmt.Sprintf("%s:%s", method, hashStr)

	return key, nil
}

// SimpleKeyGenerator generates keys using only the method name
// This is useful for methods that don't have request parameters
type SimpleKeyGenerator struct{}

// NewSimpleKeyGenerator creates a new simple key generator
func NewSimpleKeyGenerator() *SimpleKeyGenerator {
	return &SimpleKeyGenerator{}
}

// GenerateKey generates a cache key using only the method name
func (g *SimpleKeyGenerator) GenerateKey(method string, req interface{}) (string, error) {
	return method, nil
}

// CustomKeyGenerator allows custom key generation logic
type CustomKeyGenerator struct {
	keyFunc func(method string, req interface{}) (string, error)
}

// NewCustomKeyGenerator creates a new custom key generator
func NewCustomKeyGenerator(keyFunc func(method string, req interface{}) (string, error)) *CustomKeyGenerator {
	return &CustomKeyGenerator{
		keyFunc: keyFunc,
	}
}

// GenerateKey generates a cache key using custom logic
func (g *CustomKeyGenerator) GenerateKey(method string, req interface{}) (string, error) {
	return g.keyFunc(method, req)
}

// MethodKeyGenerator generates keys for specific methods with custom logic
type MethodKeyGenerator struct {
	defaultGen KeyGenerator
	methodGens map[string]KeyGenerator
}

// NewMethodKeyGenerator creates a new method-specific key generator
func NewMethodKeyGenerator(defaultGen KeyGenerator) *MethodKeyGenerator {
	if defaultGen == nil {
		defaultGen = NewDefaultKeyGenerator()
	}

	return &MethodKeyGenerator{
		defaultGen: defaultGen,
		methodGens: make(map[string]KeyGenerator),
	}
}

// RegisterMethod registers a custom key generator for a specific method
func (g *MethodKeyGenerator) RegisterMethod(method string, gen KeyGenerator) {
	g.methodGens[method] = gen
}

// GenerateKey generates a cache key using method-specific or default logic
func (g *MethodKeyGenerator) GenerateKey(method string, req interface{}) (string, error) {
	if gen, ok := g.methodGens[method]; ok {
		return gen.GenerateKey(method, req)
	}
	return g.defaultGen.GenerateKey(method, req)
}

// ExtractKeyFromInfo extracts relevant information for cache key generation
func ExtractKeyFromInfo(info *grpc.UnaryServerInfo, req interface{}) (string, error) {
	gen := NewDefaultKeyGenerator()
	return gen.GenerateKey(info.FullMethod, req)
}
