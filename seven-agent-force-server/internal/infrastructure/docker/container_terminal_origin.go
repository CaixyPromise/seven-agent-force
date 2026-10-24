package docker

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

// validateContainerTerminalOrigin mirrors coder/websocket's OriginPatterns matching
// and runs before Docker exec creation so rejected handshakes have no container side effect.
func validateContainerTerminalOrigin(request *http.Request, allowedPatterns []string) error {
	if request == nil {
		return apperrors.Params("容器终端 WebSocket 请求不能为空")
	}

	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return apperrors.Forbidden("容器终端 WebSocket 必须携带 Origin")
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return apperrors.Forbidden("容器终端 WebSocket Origin 无效")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return apperrors.Forbidden("容器终端 WebSocket Origin 协议不被允许")
	}

	if strings.EqualFold(strings.TrimSpace(request.Host), parsed.Host) {
		return nil
	}

	for _, rawPattern := range allowedPatterns {
		pattern := strings.TrimSpace(rawPattern)
		if pattern == "" {
			continue
		}
		target := parsed.Host
		if strings.Contains(pattern, "://") {
			target = parsed.Scheme + "://" + parsed.Host
		}
		matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(target))
		if err != nil {
			return apperrors.Forbidden("容器终端 WebSocket Origin 配置无效")
		}
		if matched {
			return nil
		}
	}

	return apperrors.Forbidden("容器终端 WebSocket Origin 不被允许")
}
