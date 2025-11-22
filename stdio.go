package main

import (
	"context"
	"encoding/json"

	"github.com/metoro-io/mcp-golang/transport"
)

// CleaningStdioTransport wraps a transport and removes null values from params
type CleaningStdioTransport struct {
	inner transport.Transport
}

// NewCleaningStdioTransport creates a new cleaning wrapper around the given transport
func NewCleaningStdioTransport(inner transport.Transport) *CleaningStdioTransport {
	return &CleaningStdioTransport{inner: inner}
}

// Start implements Transport.Start
func (t *CleaningStdioTransport) Start(ctx context.Context) error {
	return t.inner.Start(ctx)
}

// Send implements Transport.Send, cleaning null values from params
func (t *CleaningStdioTransport) Send(ctx context.Context, message *transport.BaseJsonRpcMessage) error {
	message = t.cleanMessage(message)
	return t.inner.Send(ctx, message)
}

// cleanMessage removes null values from request params
func (t *CleaningStdioTransport) cleanMessage(msg *transport.BaseJsonRpcMessage) *transport.BaseJsonRpcMessage {
	if msg == nil || msg.JsonRpcRequest == nil || len(msg.JsonRpcRequest.Params) == 0 {
		return msg
	}

	// Parse params, remove nulls, re-marshal
	var params map[string]interface{}
	if err := json.Unmarshal(msg.JsonRpcRequest.Params, &params); err != nil {
		return msg
	}

	// Remove null values
	cleaned := make(map[string]interface{})
	for k, v := range params {
		if v != nil {
			cleaned[k] = v
		}
	}

	// If empty after cleaning, set params to empty object
	if len(cleaned) == 0 {
		msg.JsonRpcRequest.Params = []byte("{}")
		return msg
	}

	// Re-marshal
	newParams, err := json.Marshal(cleaned)
	if err != nil {
		return msg
	}

	msg.JsonRpcRequest.Params = newParams
	return msg
}

// Close implements Transport.Close
func (t *CleaningStdioTransport) Close() error {
	return t.inner.Close()
}

// SetCloseHandler implements Transport.SetCloseHandler
func (t *CleaningStdioTransport) SetCloseHandler(handler func()) {
	t.inner.SetCloseHandler(handler)
}

// SetErrorHandler implements Transport.SetErrorHandler
func (t *CleaningStdioTransport) SetErrorHandler(handler func(error)) {
	t.inner.SetErrorHandler(handler)
}

// SetMessageHandler implements Transport.SetMessageHandler
func (t *CleaningStdioTransport) SetMessageHandler(handler func(ctx context.Context, message *transport.BaseJsonRpcMessage)) {
	t.inner.SetMessageHandler(handler)
}
