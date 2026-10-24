package static

import (
	"context"
	"errors"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
)

func TestResolverNormalizesConfiguredURLs(t *testing.T) {
	resolver, err := NewResolver(map[string][]string{"hub": {"https://hub-a.example:9443", "http://127.0.0.1:8080"}})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := resolver.Resolve(context.Background(), "hub")
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 2 || instances[0].Scheme != "https" || instances[0].Host != "hub-a.example" || instances[0].Port != 9443 {
		t.Fatalf("Resolve() = %#v", instances)
	}
}

func TestNilResolverReturnsDependencyError(t *testing.T) {
	var resolver *Resolver
	_, err := resolver.Resolve(context.Background(), "hub")
	if !errors.Is(err, microservice.ErrInvalidDependency) {
		t.Fatalf("Resolve() error = %v, want ErrInvalidDependency", err)
	}
}

func TestResolverRejectsUnsafeURLs(t *testing.T) {
	for _, rawURL := range []string{"ftp://host:21", "http://user:pass@host:80", "http://host:80/path", "http://host"} {
		if _, err := NewResolver(map[string][]string{"hub": {rawURL}}); err == nil {
			t.Fatalf("NewResolver(%q) succeeded", rawURL)
		}
	}
}
