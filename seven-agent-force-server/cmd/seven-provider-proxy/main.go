package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	providerproxy "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/proxy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo"
)

const maxSpecBytes = 1 << 20

var errControlSocketNotPrivate = errors.New("provider proxy control socket is not private")

func main() {
	if len(os.Args) > 1 && os.Args[1] == "control" {
		socketPath, err := controlSocketPath(os.Args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := controlJSON(socketPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// The control socket is created inside a Worker-owned bind mount. Keep the
	// process-wide umask private so Docker Desktop filesystems that cannot chmod
	// a mounted Unix socket still create it without group/other permissions.
	syscall.Umask(0o077)
	var specPath string
	var listenAddress string
	var showVersion bool
	flag.StringVar(&specPath, "spec", strings.TrimSpace(os.Getenv("SEVEN_PROVIDER_PROXY_SPEC")), "path to the provider proxy specification")
	flag.StringVar(&listenAddress, "listen", "0.0.0.0:8080", "private container listen address")
	flag.BoolVar(&showVersion, "version", false, "print build metadata and exit")
	flag.Parse()
	if showVersion {
		fmt.Println(buildinfo.String())
		return
	}

	config, err := loadConfig(specPath)
	if err != nil {
		slog.Error("provider proxy configuration is invalid", "error", err)
		os.Exit(2)
	}
	handler, err := providerproxy.NewHandler(config, nil)
	if err != nil {
		slog.Error("provider proxy configuration is invalid", "error", err)
		os.Exit(2)
	}

	readTimeout := 2 * time.Minute
	if config.RequestTimeoutMS > 0 {
		readTimeout = time.Duration(config.RequestTimeoutMS) * time.Millisecond
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       readTimeout,
		// Responses may stream for the lifetime of an Attempt, so an HTTP
		// WriteTimeout would terminate legitimate Codex SSE responses.
		IdleTimeout:    2 * time.Minute,
		MaxHeaderBytes: 4 << 20,
	}
	listener, cleanupListener, err := openHTTPListener(listenAddress)
	if err != nil {
		slog.Error("provider proxy listen failed", "error", err)
		os.Exit(2)
	}
	defer cleanupListener()
	control, err := startControlSocket(config.ControlSocket, handler)
	if err != nil {
		slog.Error("provider proxy control socket failed", "error", err)
		os.Exit(2)
	}
	defer control.Close()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	slog.Info("provider proxy started", "listen", listenAddress, "taskId", config.TaskID, "attemptId", config.AttemptID, "reviewRunId", config.ReviewRunID)
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("provider proxy stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

func controlSocketPath(args []string) (string, error) {
	path := strings.TrimSpace(os.Getenv("SEVEN_PROVIDER_PROXY_CONTROL_SOCKET"))
	if path == "" {
		path = "/run/seven/control/provider.sock"
	}
	for index := 1; index < len(args); index++ {
		switch {
		case args[index] == "--socket":
			if index+1 >= len(args) {
				return "", errors.New("--socket requires a path")
			}
			path = strings.TrimSpace(args[index+1])
			index++
		case strings.HasPrefix(args[index], "--socket="):
			path = strings.TrimSpace(strings.TrimPrefix(args[index], "--socket="))
		default:
			return "", errors.New("unknown provider proxy control option")
		}
	}
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("provider proxy control socket must be an absolute path")
	}
	return path, nil
}

// openHTTPListener supports the ordinary Agent Attempt TCP listener and the
// ReviewRun-only Unix listener. The latter is mounted read-only into the
// Reviewer container, so the Reviewer has no network route to the Provider
// Proxy or to the Framework Server.
func openHTTPListener(address string) (net.Listener, func(), error) {
	address = strings.TrimSpace(address)
	if strings.HasPrefix(address, "unix://") {
		path := strings.TrimPrefix(address, "unix://")
		if path == "" || !filepath.IsAbs(path) {
			return nil, func() {}, errors.New("unix listen path must be absolute")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, func() {}, err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, func() {}, err
		}
		listener, err := net.Listen("unix", path)
		if err != nil {
			return nil, func() {}, err
		}
		if err := ensurePrivateSocket(path); err != nil {
			_ = listener.Close()
			_ = os.Remove(path)
			return nil, func() {}, err
		}
		return listener, func() {
			_ = listener.Close()
			_ = os.Remove(path)
		}, nil
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, func() {}, err
	}
	return listener, func() { _ = listener.Close() }, nil
}

// controlJSON is executed through docker exec. It keeps the Provider Proxy
// control plane on the container-private UDS because Docker Desktop exposes a
// bind-mounted socket inode to the host but does not reliably forward host
// AF_UNIX connections to the VM. No TCP or host port is introduced.
func controlJSON(socketPath string) error {
	body, err := io.ReadAll(io.LimitReader(os.Stdin, 16<<10+1))
	if err != nil {
		return err
	}
	if len(body) == 0 || len(body) > 16<<10 {
		return errors.New("control request must be between 1 byte and 16 KiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var request controlRequest
	if err = decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("control request is invalid")
	}
	connection, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	if err = json.NewEncoder(connection).Encode(request); err != nil {
		return err
	}
	response, err := bufio.NewReader(io.LimitReader(connection, 16<<10+1)).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(response) == 0 || len(response) > 16<<10 {
		return errors.New("control response is invalid")
	}
	_, err = os.Stdout.Write(response)
	return err
}

type controlServer struct {
	listener net.Listener
}

func (s *controlServer) Close() error { return s.listener.Close() }

type controlRequest struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Command                 string `json:"command"`
	AttemptID               int64  `json:"attemptId,string"`
	Generation              int64  `json:"generation,string"`
	ExpectedEpoch           int64  `json:"expectedOwnerEpoch,string,omitempty"`
	OwnerEpoch              int64  `json:"ownerEpoch,string"`
	WorkerInstanceID        string `json:"workerInstanceId,omitempty"`
	LeaseToken              string `json:"leaseToken,omitempty"`
	LeaseExpiresAt          string `json:"leaseExpiresAt,omitempty"`
	ReattachPermitID        int64  `json:"reattachPermitId,string,omitempty"`
	ReattachPermitToken     string `json:"reattachPermitToken,omitempty"`
	ReattachPermitExpiresAt string `json:"reattachPermitExpiresAt,omitempty"`
}

func startControlSocket(path string, handler *providerproxy.Handler) (*controlServer, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("provider proxy controlSocket is required")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := ensurePrivateSocket(path); err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &controlServer{listener: listener}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go handleControl(connection, handler)
		}
	}()
	return server, nil
}

func ensurePrivateSocket(path string) error {
	return ensurePrivateSocketWithChmod(path, os.Chmod)
}

func ensurePrivateSocketWithChmod(path string, chmod func(string, os.FileMode) error) error {
	if err := chmod(path, 0o600); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.EPERM) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errControlSocketNotPrivate
	}
	if info.Mode().Perm()&0o077 == 0 {
		return nil
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm()&0o077 != 0 {
		return errControlSocketNotPrivate
	}
	return nil
}

func handleControl(connection net.Conn, handler *providerproxy.Handler) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	line, err := bufio.NewReader(io.LimitReader(connection, (16<<10)+1)).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		_ = json.NewEncoder(connection).Encode(map[string]any{"ok": false, "reasonCode": "INVALID_CONTROL_REQUEST"})
		return
	}
	if len(line) == 0 || len(line) > 16<<10 {
		_ = json.NewEncoder(connection).Encode(map[string]any{"ok": false, "reasonCode": "INVALID_CONTROL_REQUEST"})
		return
	}
	decoder := json.NewDecoder(strings.NewReader(string(line)))
	decoder.DisallowUnknownFields()
	var request controlRequest
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		_ = json.NewEncoder(connection).Encode(map[string]any{"ok": false, "reasonCode": "INVALID_CONTROL_REQUEST"})
		return
	}
	if request.SchemaVersion != 1 || !handler.MatchesIdentity(request.AttemptID, request.Generation) {
		_ = json.NewEncoder(connection).Encode(map[string]any{"ok": false, "reasonCode": "IDENTITY_MISMATCH"})
		return
	}
	state := handler.Authority()
	var commandErr error
	switch request.Command {
	case "state", "diagnostics":
	case "adopt-owner":
		commandErr = handler.AdoptOwner(request.ExpectedEpoch, request.OwnerEpoch, request.WorkerInstanceID, request.LeaseToken, request.LeaseExpiresAt, request.ReattachPermitID, request.ReattachPermitToken, request.ReattachPermitExpiresAt)
	case "renew-authority":
		commandErr = handler.RenewAuthority(request.OwnerEpoch, request.LeaseToken, request.LeaseExpiresAt)
	case "interrupt":
		handler.Interrupt()
	default:
		commandErr = errors.New("UNKNOWN_CONTROL_COMMAND")
	}
	if commandErr != nil {
		_ = json.NewEncoder(connection).Encode(map[string]any{"ok": false, "reasonCode": commandErr.Error()})
		return
	}
	state = handler.Authority()
	_ = json.NewEncoder(connection).Encode(map[string]any{"ok": true, "ownerEpoch": fmt.Sprintf("%d", state.OwnerEpoch), "workerInstanceId": state.WorkerInstanceID, "leaseExpiresAt": state.LeaseExpiresAt, "reattachPermitId": fmt.Sprintf("%d", state.ReattachPermitID), "reattachPermitDigest": state.ReattachPermitDigest, "controlState": "READY"})
}

func loadConfig(path string) (providerproxy.Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return providerproxy.Config{}, errors.New("--spec is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return providerproxy.Config{}, fmt.Errorf("open specification: %w", err)
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxSpecBytes+1))
	if err != nil {
		return providerproxy.Config{}, fmt.Errorf("read specification: %w", err)
	}
	if len(body) > maxSpecBytes {
		return providerproxy.Config{}, errors.New("specification exceeds 1 MiB")
	}
	var config providerproxy.Config
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return providerproxy.Config{}, fmt.Errorf("decode specification: %w", err)
	}
	return config, nil
}
