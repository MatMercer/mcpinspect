package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/metoro-io/mcp-golang/transport"
)

// SSEClientTransport implements a client-side HTTP transport that handles SSE responses
type SSEClientTransport struct {
	baseURL        string
	messageHandler func(ctx context.Context, message *transport.BaseJsonRpcMessage)
	errorHandler   func(error)
	closeHandler   func()
	mu             sync.RWMutex
	client         *http.Client
	headers        map[string]string
	sessionID      string // MCP session ID from server
}

// NewSSEClientTransport creates a new SSE-aware HTTP client transport
func NewSSEClientTransport(baseURL string) *SSEClientTransport {
	return &SSEClientTransport{
		baseURL: baseURL,
		client:  &http.Client{},
		headers: make(map[string]string),
	}
}

// WithHeader adds a header to requests
func (t *SSEClientTransport) WithHeader(key, value string) *SSEClientTransport {
	t.headers[key] = value
	return t
}

// Start implements Transport.Start
func (t *SSEClientTransport) Start(ctx context.Context) error {
	return nil
}

// Send implements Transport.Send
func (t *SSEClientTransport) Send(ctx context.Context, message *transport.BaseJsonRpcMessage) error {
	// Clean up null values from params (workaround for library sending cursor: null)
	message = t.cleanMessage(message)

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}

	// Add session ID if we have one
	t.mu.RLock()
	sessionID := t.sessionID
	t.mu.RUnlock()
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Capture session ID from response
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.mu.Lock()
		t.sessionID = sid
		t.mu.Unlock()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned error: %s (status: %d)", string(body), resp.StatusCode)
	}

	// Check if response is SSE
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		return t.parseSSEResponse(ctx, resp.Body)
	}

	// Regular JSON response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if len(body) > 0 {
		return t.handleJSONResponse(ctx, body)
	}

	return nil
}

func (t *SSEClientTransport) parseSSEResponse(ctx context.Context, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		} else if line == "" && len(dataLines) > 0 {
			// End of event, process accumulated data
			data := strings.Join(dataLines, "\n")
			if err := t.handleJSONResponse(ctx, []byte(data)); err != nil {
				return err
			}
			dataLines = nil
		}
	}

	// Handle any remaining data
	if len(dataLines) > 0 {
		data := strings.Join(dataLines, "\n")
		return t.handleJSONResponse(ctx, []byte(data))
	}

	return scanner.Err()
}

func (t *SSEClientTransport) handleJSONResponse(ctx context.Context, body []byte) error {
	t.mu.RLock()
	handler := t.messageHandler
	t.mu.RUnlock()

	if handler == nil {
		return nil
	}

	// Try to unmarshal as response
	var response transport.BaseJSONRPCResponse
	if err := json.Unmarshal(body, &response); err == nil && response.Jsonrpc != "" {
		handler(ctx, transport.NewBaseMessageResponse(&response))
		return nil
	}

	// Try as error
	var errorResponse transport.BaseJSONRPCError
	if err := json.Unmarshal(body, &errorResponse); err == nil && errorResponse.Jsonrpc != "" {
		handler(ctx, transport.NewBaseMessageError(&errorResponse))
		return nil
	}

	// Try as notification
	var notification transport.BaseJSONRPCNotification
	if err := json.Unmarshal(body, &notification); err == nil && notification.Jsonrpc != "" {
		handler(ctx, transport.NewBaseMessageNotification(&notification))
		return nil
	}

	// Try as request
	var request transport.BaseJSONRPCRequest
	if err := json.Unmarshal(body, &request); err == nil && request.Jsonrpc != "" {
		handler(ctx, transport.NewBaseMessageRequest(&request))
		return nil
	}

	return fmt.Errorf("received invalid response: %s", string(body))
}

// cleanMessage removes null values from request params
func (t *SSEClientTransport) cleanMessage(msg *transport.BaseJsonRpcMessage) *transport.BaseJsonRpcMessage {
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

	// Re-marshal
	newParams, err := json.Marshal(cleaned)
	if err != nil {
		return msg
	}

	msg.JsonRpcRequest.Params = newParams
	return msg
}

// Close implements Transport.Close
func (t *SSEClientTransport) Close() error {
	if t.closeHandler != nil {
		t.closeHandler()
	}
	return nil
}

// SetCloseHandler implements Transport.SetCloseHandler
func (t *SSEClientTransport) SetCloseHandler(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeHandler = handler
}

// SetErrorHandler implements Transport.SetErrorHandler
func (t *SSEClientTransport) SetErrorHandler(handler func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorHandler = handler
}

// SetMessageHandler implements Transport.SetMessageHandler
func (t *SSEClientTransport) SetMessageHandler(handler func(ctx context.Context, message *transport.BaseJsonRpcMessage)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageHandler = handler
}
