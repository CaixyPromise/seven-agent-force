package websocket

import "sync/atomic"

var activeConnections atomic.Int64

func ActiveConnections() int64 {
	return activeConnections.Load()
}
