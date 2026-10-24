package websocket

type Result[T any] struct {
	Value T
	Err   error
}

type outboundMessage struct {
	typ     messageType
	payload []byte
}

type messageType int

const (
	messageTypeText messageType = iota + 1
	messageTypeBinary
)
