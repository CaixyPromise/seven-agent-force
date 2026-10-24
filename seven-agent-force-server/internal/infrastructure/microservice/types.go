package microservice

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var (
	ErrNoHealthyInstance       = errors.New("no healthy service instance")
	ErrInvalidRequest          = errors.New("invalid service request")
	ErrRequestTooLarge         = errors.New("service request exceeds limit")
	ErrResponseTooLarge        = errors.New("service response exceeds limit")
	ErrRegistrationUnsupported = errors.New("service registration is unsupported")
	ErrInvalidDependency       = errors.New("invalid microservice dependency")
	ErrInvalidContext          = errors.New("invalid context")
	ErrManagerStarted          = errors.New("microservice manager already started")
	ErrManagerShutdown         = errors.New("microservice manager is shut down")
)

type ServiceInstance struct {
	ID          string
	ServiceName string
	Host        string
	Port        int
	Scheme      string
	Healthy     bool
	Tags        []string
	Metadata    map[string]string
	dialIP      string
}

func (i ServiceInstance) BaseURL() string {
	return (&url.URL{Scheme: i.Scheme, Host: net.JoinHostPort(i.Host, strconv.Itoa(i.Port))}).String()
}

func (i ServiceInstance) IdentityKey() string {
	return i.ID + "|" + strings.ToLower(i.Scheme) + "|" + strings.ToLower(net.JoinHostPort(i.Host, strconv.Itoa(i.Port)))
}

type ServiceRegistration struct {
	ID              string
	ServiceName     string
	Address         string
	Port            int
	Scheme          string
	Tags            []string
	Metadata        map[string]string
	HealthCheckPath string
	HealthInterval  time.Duration
	HealthTimeout   time.Duration
}

// TraceFailureHandler receives a safe operation name when optional trace correlation degrades.
// Implementations must not alter the request outcome.
type TraceFailureHandler func(operation string)

type ServiceRequest struct {
	ServiceName       string
	ResolvedInstances []ServiceInstance
	Method            string
	Path              string
	Header            http.Header
	Body              []byte
	ReplaySafe        bool
	// TracePropagation is reserved for trusted in-platform calls such as Hub to Node.
	// External HTTP calls keep their existing wire contract unless this is explicitly enabled.
	TracePropagation bool
	// TraceWarning observes a failure in optional trace correlation for a trusted request.
	// The client isolates this callback so a warning failure cannot affect the request path.
	TraceWarning     TraceFailureHandler
	MaxResponseBytes int64
	TrustScope       TrustScope
}

type ServiceResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	InstanceID string
}

func ParseServiceURL(serviceName, rawURL string) (ServiceInstance, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ServiceInstance{}, fmt.Errorf("parse static service URL: %w", ErrInvalidRequest)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ServiceInstance{}, fmt.Errorf("unsupported service URL scheme: %w", ErrInvalidRequest)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return ServiceInstance{}, fmt.Errorf("service URL must contain only scheme, host and port: %w", ErrInvalidRequest)
	}
	portText := u.Port()
	if portText == "" {
		return ServiceInstance{}, fmt.Errorf("service URL requires an explicit port: %w", ErrInvalidRequest)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return ServiceInstance{}, fmt.Errorf("invalid service URL port: %w", ErrInvalidRequest)
	}
	return ServiceInstance{
		ID:          serviceName + "@" + u.Host,
		ServiceName: serviceName,
		Host:        u.Hostname(),
		Port:        port,
		Scheme:      u.Scheme,
		Healthy:     true,
	}, nil
}
