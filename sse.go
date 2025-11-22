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

// TraditionalSSETransport implements client-side traditional SSE transport for MCP.
// Traditional SSE flow:
// 1. Client GETs /sse to establish SSE stream
// 2. Server sends "endpoint" event with POST URL
// 3. Client POSTs messages to that endpoint
// 4. Responses come via SSE stream
type TraditionalSSETransport struct {
	sseURL         string
	postEndpoint   string
	messageHandler func(ctx context.Context, message *transport.BaseJsonRpcMessage)
	errorHandler   func(error)
	closeHandler   func()
	mu             sync.RWMutex
	client         *http.Client
	headers        map[string]string
	sseResp        *http.Response
	started        bool
}

// NewTraditionalSSETransport creates a new traditional SSE client transport
func NewTraditionalSSETransport(sseURL string) *TraditionalSSETransport {
	return &TraditionalSSETransport{
		sseURL:  sseURL,
		client:  &http.Client{},
		headers: make(map[string]string),
	}
}

// WithHeader adds a header to requests
func (t *TraditionalSSETransport) WithHeader(key, value string) *TraditionalSSETransport {
	t.headers[key] = value
	return t
}

// Start establishes the SSE connection and waits for the endpoint event
func (t *TraditionalSSETransport) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return nil // Already started, idempotent
	}
	t.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.sseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to SSE endpoint: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return fmt.Errorf("SSE connection failed: %s (status: %d)", string(body), resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		resp.Body.Close()
		return fmt.Errorf("unexpected content type: %s", contentType)
	}

	t.mu.Lock()
	t.sseResp = resp
	t.started = true
	t.mu.Unlock()

	// Wait for endpoint event
	endpointCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go t.readSSEEvents(ctx, resp.Body, endpointCh, errCh)

	select {
	case endpoint := <-endpointCh:
		t.mu.Lock()
		t.postEndpoint = t.resolveEndpoint(endpoint)
		t.mu.Unlock()
		return nil
	case err := <-errCh:
		return fmt.Errorf("failed to get endpoint: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// resolveEndpoint converts relative endpoint to absolute URL
func (t *TraditionalSSETransport) resolveEndpoint(endpoint string) string {
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}

	// Extract base URL from sseURL
	base := t.sseURL
	if idx := strings.LastIndex(base, "/"); idx > 8 { // after "http://" or "https://"
		base = base[:idx]
	}
	if strings.HasPrefix(endpoint, "/") {
		return base + endpoint
	}
	return base + "/" + endpoint
}

// readSSEEvents reads SSE events from the stream
func (t *TraditionalSSETransport) readSSEEvents(ctx context.Context, reader io.Reader, endpointCh chan<- string, errCh chan<- error) {
	scanner := bufio.NewScanner(reader)
	var eventType string
	var dataLines []string
	endpointSent := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		} else if line == "" && len(dataLines) > 0 {
			// End of event
			data := strings.Join(dataLines, "\n")

			if eventType == "endpoint" && !endpointSent {
				endpointCh <- data
				endpointSent = true
			} else if eventType == "message" || eventType == "" {
				t.handleMessage(ctx, []byte(data))
			}

			eventType = ""
			dataLines = nil
		}
	}

	if err := scanner.Err(); err != nil && !endpointSent {
		errCh <- err
	}

	t.mu.RLock()
	closeHandler := t.closeHandler
	t.mu.RUnlock()
	if closeHandler != nil {
		closeHandler()
	}
}

// handleMessage processes a JSON-RPC message from SSE
func (t *TraditionalSSETransport) handleMessage(ctx context.Context, data []byte) {
	t.mu.RLock()
	handler := t.messageHandler
	t.mu.RUnlock()

	if handler == nil {
		return
	}

	// Try as response
	var response transport.BaseJSONRPCResponse
	if err := json.Unmarshal(data, &response); err == nil && response.Jsonrpc != "" {
		handler(ctx, transport.NewBaseMessageResponse(&response))
		return
	}

	// Try as error
	var errorResponse transport.BaseJSONRPCError
	if err := json.Unmarshal(data, &errorResponse); err == nil && errorResponse.Jsonrpc != "" {
		handler(ctx, transport.NewBaseMessageError(&errorResponse))
		return
	}

	// Try as notification
	var notification transport.BaseJSONRPCNotification
	if err := json.Unmarshal(data, &notification); err == nil && notification.Jsonrpc != "" {
		handler(ctx, transport.NewBaseMessageNotification(&notification))
		return
	}

	// Try as request
	var request transport.BaseJSONRPCRequest
	if err := json.Unmarshal(data, &request); err == nil && request.Jsonrpc != "" {
		handler(ctx, transport.NewBaseMessageRequest(&request))
		return
	}
}

// Send sends a JSON-RPC message via POST to the endpoint
func (t *TraditionalSSETransport) Send(ctx context.Context, message *transport.BaseJsonRpcMessage) error {
	t.mu.RLock()
	endpoint := t.postEndpoint
	t.mu.RUnlock()

	if endpoint == "" {
		return fmt.Errorf("transport not started or endpoint not received")
	}

	// Clean up null values from params
	message = t.cleanMessage(message)

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// For traditional SSE, POST typically returns 200/202 with empty body
	// Actual response comes via SSE stream
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned error: %s (status: %d)", string(body), resp.StatusCode)
	}

	return nil
}

// Close closes the SSE connection
func (t *TraditionalSSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.sseResp != nil {
		t.sseResp.Body.Close()
		t.sseResp = nil
	}

	if t.closeHandler != nil {
		t.closeHandler()
	}

	return nil
}

// SetCloseHandler sets the close callback
func (t *TraditionalSSETransport) SetCloseHandler(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeHandler = handler
}

// SetErrorHandler sets the error callback
func (t *TraditionalSSETransport) SetErrorHandler(handler func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorHandler = handler
}

// SetMessageHandler sets the message callback
func (t *TraditionalSSETransport) SetMessageHandler(handler func(ctx context.Context, message *transport.BaseJsonRpcMessage)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageHandler = handler
}

// cleanMessage removes null values from request params
func (t *TraditionalSSETransport) cleanMessage(msg *transport.BaseJsonRpcMessage) *transport.BaseJsonRpcMessage {
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
