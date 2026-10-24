package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	RepositoryProviderGitHub  = "GITHUB"
	RepositoryProviderGitLab  = "GITLAB"
	RepositoryProviderGitea   = "GITEA"
	RepositoryProviderGeneric = "GENERIC_GIT"

	RepositorySourcePreparing = "PREPARING"
	RepositorySourceReady     = "READY"
	RepositorySourceFailed    = "FAILED"
)

var (
	ErrRepositoryURLInvalid         = errors.New("PROJECT_REPOSITORY_URL_INVALID")
	ErrRepositoryCredentialRequired = errors.New("PROJECT_REPOSITORY_CREDENTIAL_REQUIRED")
	ErrRepositoryAuthentication     = errors.New("PROJECT_REPOSITORY_AUTHENTICATION_FAILED")
	ErrRepositoryNotFound           = errors.New("PROJECT_REPOSITORY_NOT_FOUND")
	ErrRepositoryRefNotFound        = errors.New("PROJECT_REPOSITORY_REF_NOT_FOUND")
	ErrRepositoryPushUnavailable    = errors.New("PROJECT_REPOSITORY_PUSH_UNAVAILABLE")
	ErrRepositoryReviewUnavailable  = errors.New("PROJECT_REPOSITORY_REVIEW_UNAVAILABLE")
	ErrRepositoryPreparationFailed  = errors.New("PROJECT_REPOSITORY_PREPARATION_FAILED")
)

type RepositoryCredential struct {
	Username string
	Token    string
}

type ParsedRepositoryURL struct {
	Scheme         string
	Host           string
	Port           int
	RepositoryPath string
	DisplayURL     string
	ProviderType   string
	Credential     RepositoryCredential
	ConfigDigest   string
}

type RepositoryCapabilities struct {
	Read        bool `json:"read"`
	Push        bool `json:"push"`
	DraftReview bool `json:"draftReview"`
}

// ParseRepositoryURL accepts a credential-bearing HTTP(S) Git URL and returns
// a safe target plus a separate in-memory credential. The returned DisplayURL
// and ConfigDigest never contain userinfo.
func ParseRepositoryURL(raw string) (ParsedRepositoryURL, error) {
	if raw == "" || len(raw) > 16<<10 || !utf8.ValidString(raw) || strings.TrimSpace(raw) != raw || containsControl(raw) {
		return ParsedRepositoryURL{}, ErrRepositoryURLInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return ParsedRepositoryURL{}, ErrRepositoryURLInvalid
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	path, err := url.PathUnescape(parsed.EscapedPath())
	path = strings.TrimPrefix(path, "/")
	if err != nil || host == "" || path == "" || path == "." || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") || containsControl(path) || len(host) > 253 || len(path) > 2048 {
		return ParsedRepositoryURL{}, ErrRepositoryURLInvalid
	}
	port := 443
	if parsed.Scheme == "http" {
		port = 80
	}
	if rawPort := parsed.Port(); rawPort != "" {
		value, convErr := strconv.Atoi(rawPort)
		if convErr != nil || value < 1 || value > 65535 {
			return ParsedRepositoryURL{}, ErrRepositoryURLInvalid
		}
		port = value
	}
	credential := RepositoryCredential{}
	if parsed.User != nil {
		credential.Username = parsed.User.Username()
		credential.Token, _ = parsed.User.Password()
		if credential.Username == "" || credential.Token == "" || len(credential.Username) > 1024 || len(credential.Token) > 8192 || containsControl(credential.Username) || containsControl(credential.Token) {
			return ParsedRepositoryURL{}, ErrRepositoryCredentialRequired
		}
	}
	displayHost := host
	if port != defaultPort(parsed.Scheme) {
		displayHost = net.JoinHostPort(host, strconv.Itoa(port))
	}
	displayURL := (&url.URL{Scheme: parsed.Scheme, Host: displayHost, Path: "/" + path}).String()
	provider := RepositoryProviderGeneric
	if host == "github.com" {
		provider = RepositoryProviderGitHub
	}
	digestInput := strings.Join([]string{parsed.Scheme, host, strconv.Itoa(port), path, credentialDigest(credential)}, "\x00")
	digest := sha256.Sum256([]byte(digestInput))
	return ParsedRepositoryURL{
		Scheme: parsed.Scheme, Host: host, Port: port, RepositoryPath: path,
		DisplayURL: displayURL, ProviderType: provider, Credential: credential,
		ConfigDigest: hex.EncodeToString(digest[:]),
	}, nil
}

func defaultPort(scheme string) int {
	if scheme == "http" {
		return 80
	}
	return 443
}

func credentialDigest(value RepositoryCredential) string {
	sum := sha256.Sum256([]byte(value.Username + "\x00" + value.Token))
	return hex.EncodeToString(sum[:])
}

func containsControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
