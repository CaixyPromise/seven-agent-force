package docker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/coder/websocket"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	defaultTerminalShell = "/bin/sh"
	defaultTerminalRows  = 24
	defaultTerminalCols  = 80
	maxTerminalRows      = 200
	maxTerminalCols      = 400
)

func NormalizeContainerTerminalRequest(request ContainerTerminalRequest) (ContainerTerminalRequest, error) {
	request.Shell = strings.TrimSpace(request.Shell)
	if request.Shell == "" {
		request.Shell = defaultTerminalShell
	}
	if request.Shell != "/bin/sh" && request.Shell != "/bin/bash" {
		return ContainerTerminalRequest{}, apperrors.Params("terminal shell 仅支持 /bin/sh 或 /bin/bash")
	}
	if request.Rows == 0 {
		request.Rows = defaultTerminalRows
	}
	if request.Cols == 0 {
		request.Cols = defaultTerminalCols
	}
	if request.Rows > maxTerminalRows {
		request.Rows = maxTerminalRows
	}
	if request.Cols > maxTerminalCols {
		request.Cols = maxTerminalCols
	}
	return request, nil
}

func (s *service) ServeContainerTerminal(ctx context.Context, writer http.ResponseWriter, request *http.Request, id string, command ContainerTerminalRequest) error {
	command, err := NormalizeContainerTerminalRequest(command)
	if err != nil {
		return err
	}
	if err := validateContainerTerminalOrigin(request, s.originPatterns); err != nil {
		return err
	}
	cli, resolved, _, err := s.resolveContainer(ctx, id)
	if err != nil {
		return err
	}
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	createResp, err := cli.ContainerExecCreate(execCtx, resolved, dockercontainer.ExecOptions{
		Cmd:          []string{command.Shell},
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Detach:       false,
	})
	if err != nil {
		return apperrors.Operation("创建 Docker 容器终端失败：" + err.Error())
	}
	attach, err := cli.ContainerExecAttach(execCtx, createResp.ID, dockercontainer.ExecAttachOptions{
		Tty:    true,
		Detach: false,
	})
	if err != nil {
		return apperrors.Operation("连接 Docker 容器终端失败：" + err.Error())
	}
	defer attach.Close()
	_ = cli.ContainerExecResize(execCtx, createResp.ID, dockercontainer.ResizeOptions{Height: command.Rows, Width: command.Cols})

	conn, err := websocket.Accept(writer, request, &websocket.AcceptOptions{
		OriginPatterns: s.originPatterns,
	})
	if err != nil {
		return apperrors.Operation("升级容器终端 WebSocket 失败：" + err.Error())
	}
	defer conn.CloseNow()

	done := make(chan struct{})
	var closeOnce sync.Once
	closeSession := func() {
		closeOnce.Do(func() {
			cancel()
			attach.Close()
			_ = conn.Close(websocket.StatusNormalClosure, "terminal closed")
			close(done)
		})
	}
	go copyTerminalDockerToWebSocket(execCtx, conn, attach.Reader, closeSession)
	go copyTerminalWebSocketToDocker(execCtx, conn, attach.Conn, closeSession)

	select {
	case <-ctx.Done():
		closeSession()
	case <-request.Context().Done():
		closeSession()
	case <-done:
	}
	return nil
}

func copyTerminalDockerToWebSocket(ctx context.Context, conn *websocket.Conn, reader io.Reader, closeSession func()) {
	defer closeSession()
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			writeErr := conn.Write(writeCtx, websocket.MessageBinary, buffer[:n])
			cancel()
			if writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func copyTerminalWebSocketToDocker(ctx context.Context, conn *websocket.Conn, writer io.Writer, closeSession func()) {
	defer closeSession()
	for {
		messageType, reader, err := conn.Reader(ctx)
		if err != nil {
			return
		}
		if messageType != websocket.MessageText && messageType != websocket.MessageBinary {
			continue
		}
		if _, err := io.Copy(writer, reader); err != nil && !errors.Is(err, io.EOF) {
			return
		}
	}
}
