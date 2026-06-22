package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	serverName            = "local-figma-mcp-bridge"
	serverVersion         = "0.1.0"
	latestProtocolVersion = "2025-11-25"
)

var supportedProtocolVersions = map[string]bool{
	"2025-11-25": true,
	"2025-06-18": true,
	"2025-03-26": true,
	"2024-11-05": true,
	"2024-10-07": true,
}

type bridgeRequest struct {
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Command string      `json:"command"`
	Payload interface{} `json:"payload,omitempty"`
}

type bridgeResponse struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type pluginConnection struct {
	conn net.Conn
	mu   sync.Mutex
}

func (c *pluginConnection) sendJSON(value interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeWebSocketFrame(c.conn, 0x1, payload)
}

type pendingRequest struct {
	client *pluginConnection
}

type bridge struct {
	host string
	port string

	mu               sync.Mutex
	plugin           *pluginConnection
	lastPluginSeenAt string
	pending          map[string]pendingRequest
}

func newBridge(host, port string) *bridge {
	return &bridge{
		host:    host,
		port:    port,
		pending: make(map[string]pendingRequest),
	}
}

func (b *bridge) websocketURL() string {
	return "ws://" + b.host + ":" + b.port
}

func (b *bridge) status() map[string]interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]interface{}{
		"websocket":        b.websocketURL(),
		"bridgeConnected":  true,
		"connected":        b.plugin != nil,
		"lastPluginSeenAt": b.lastPluginSeenAt,
	}
}

func (b *bridge) listen(ctx context.Context) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(b.host, b.port))
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go b.handleConnection(conn)
	}
}

func (b *bridge) handleConnection(conn net.Conn) {
	path, err := acceptWebSocket(conn)
	if err != nil {
		_ = conn.Close()
		return
	}

	plugin := &pluginConnection{conn: conn}
	if path == "/mcp" {
		b.handleController(plugin)
		return
	}

	b.handlePlugin(plugin)
}

func (b *bridge) handlePlugin(plugin *pluginConnection) {
	b.mu.Lock()
	b.plugin = plugin
	b.lastPluginSeenAt = time.Now().UTC().Format(time.RFC3339)
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		if b.plugin == plugin {
			b.plugin = nil
		}
		b.mu.Unlock()
		_ = plugin.conn.Close()
	}()

	for {
		opcode, payload, err := readWebSocketFrame(plugin.conn)
		if err != nil {
			return
		}

		switch opcode {
		case 0x1:
			b.handlePluginMessage(payload)
		case 0x8:
			return
		case 0x9:
			plugin.mu.Lock()
			_ = writeWebSocketFrame(plugin.conn, 0xA, payload)
			plugin.mu.Unlock()
		}
	}
}

func (b *bridge) handleController(client *pluginConnection) {
	defer func() {
		b.removeControllerPending(client)
		_ = client.conn.Close()
	}()

	for {
		opcode, payload, err := readWebSocketFrame(client.conn)
		if err != nil {
			return
		}

		switch opcode {
		case 0x1:
			b.handleControllerMessage(client, payload)
		case 0x8:
			return
		case 0x9:
			client.mu.Lock()
			_ = writeWebSocketFrame(client.conn, 0xA, payload)
			client.mu.Unlock()
		}
	}
}

func (b *bridge) handleControllerMessage(client *pluginConnection, payload []byte) {
	var request bridgeRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return
	}

	switch request.Type {
	case "status":
		client.sendJSON(bridgeResponse{
			ID:      request.ID,
			Type:    "result",
			Payload: mustRawJSON(b.status()),
		})
	case "command":
		b.forwardCommand(client, request)
	}
}

func (b *bridge) forwardCommand(client *pluginConnection, request bridgeRequest) {
	b.mu.Lock()
	plugin := b.plugin
	if plugin == nil {
		b.mu.Unlock()
		client.sendJSON(bridgeResponse{
			ID:    request.ID,
			Type:  "error",
			Error: "Figma plugin is not connected. Run the Local Figma MCP Bridge plugin in Figma.",
		})
		return
	}
	b.pending[request.ID] = pendingRequest{client: client}
	b.mu.Unlock()

	if err := plugin.sendJSON(request); err != nil {
		b.mu.Lock()
		delete(b.pending, request.ID)
		b.mu.Unlock()
		client.sendJSON(bridgeResponse{
			ID:    request.ID,
			Type:  "error",
			Error: err.Error(),
		})
	}
}

func (b *bridge) removeControllerPending(client *pluginConnection) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, pending := range b.pending {
		if pending.client == client {
			delete(b.pending, id)
		}
	}
}

func (b *bridge) handlePluginMessage(payload []byte) {
	var response bridgeResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return
	}
	if response.Type != "result" && response.Type != "error" {
		return
	}

	b.mu.Lock()
	pending, ok := b.pending[response.ID]
	if ok {
		delete(b.pending, response.ID)
	}
	b.mu.Unlock()

	if !ok {
		return
	}

	pending.client.sendJSON(response)
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type figmaGateway interface {
	status() (interface{}, error)
	sendToPlugin(command string, payload interface{}) (json.RawMessage, error)
}

type bridgeClient struct {
	host string
	port string
}

func newBridgeClient(host, port string) *bridgeClient {
	return &bridgeClient{host: host, port: port}
}

func (c *bridgeClient) websocketURL() string {
	return "ws://" + c.host + ":" + c.port
}

func (c *bridgeClient) status() (interface{}, error) {
	response, err := c.sendBridgeRequest(bridgeRequest{Type: "status"}, 5*time.Second)
	if err != nil {
		return map[string]interface{}{
			"websocket":       c.websocketURL(),
			"bridgeConnected": false,
			"connected":       false,
			"error":           err.Error(),
		}, nil
	}

	var value interface{}
	if len(response.Payload) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(response.Payload, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (c *bridgeClient) sendToPlugin(command string, payload interface{}) (json.RawMessage, error) {
	response, err := c.sendBridgeRequest(bridgeRequest{
		Type:    "command",
		Command: command,
		Payload: payload,
	}, 20*time.Second)
	if err != nil {
		return nil, err
	}
	if response.Type == "error" {
		if response.Error == "" {
			response.Error = "Figma plugin returned an error."
		}
		return nil, errors.New(response.Error)
	}
	if len(response.Payload) == 0 {
		return json.RawMessage("null"), nil
	}
	return response.Payload, nil
}

func (c *bridgeClient) sendBridgeRequest(request bridgeRequest, timeout time.Duration) (bridgeResponse, error) {
	id, err := randomID()
	if err != nil {
		return bridgeResponse{}, err
	}
	request.ID = id

	conn, err := dialWebSocket(c.host, c.port, "/mcp")
	if err != nil {
		return bridgeResponse{}, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return bridgeResponse{}, err
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return bridgeResponse{}, err
	}
	if err := writeWebSocketFrame(conn, 0x1, payload); err != nil {
		return bridgeResponse{}, err
	}

	for {
		opcode, payload, err := readWebSocketFrame(conn)
		if err != nil {
			return bridgeResponse{}, err
		}
		switch opcode {
		case 0x1:
			var response bridgeResponse
			if err := json.Unmarshal(payload, &response); err != nil {
				return bridgeResponse{}, err
			}
			if response.ID == id {
				return response, nil
			}
		case 0x8:
			return bridgeResponse{}, errors.New("bridge closed the websocket connection")
		case 0x9:
			_ = writeWebSocketFrame(conn, 0xA, payload)
		}
	}
}

func serveMCP(ctx context.Context, gateway figmaGateway, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var request rpcRequest
		if err := json.Unmarshal(line, &request); err != nil {
			writeRPC(writer, rpcResponse{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "Parse error"},
			})
			continue
		}

		if len(request.ID) == 0 {
			continue
		}

		response := handleRPC(ctx, gateway, request)
		writeRPC(writer, response)
	}
}

func handleRPC(ctx context.Context, gateway figmaGateway, request rpcRequest) rpcResponse {
	response := rpcResponse{JSONRPC: "2.0", ID: request.ID}

	switch request.Method {
	case "initialize":
		protocolVersion := latestProtocolVersion
		var params initializeParams
		if err := json.Unmarshal(request.Params, &params); err == nil && supportedProtocolVersions[params.ProtocolVersion] {
			protocolVersion = params.ProtocolVersion
		}
		response.Result = map[string]interface{}{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    serverName,
				"version": serverVersion,
			},
		}
	case "ping":
		response.Result = map[string]interface{}{}
	case "tools/list":
		response.Result = map[string]interface{}{"tools": tools()}
	case "tools/call":
		result, err := callTool(ctx, gateway, request.Params)
		if err != nil {
			response.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			response.Result = result
		}
	default:
		response.Error = &rpcError{Code: -32601, Message: "Method not found"}
	}

	return response
}

func callTool(_ context.Context, gateway figmaGateway, rawParams json.RawMessage) (interface{}, error) {
	var params toolCallParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, err
	}
	if params.Arguments == nil {
		params.Arguments = map[string]interface{}{}
	}

	switch params.Name {
	case "figma_status":
		status, err := gateway.status()
		if err != nil {
			return nil, err
		}
		return textResult(status), nil
	case "figma_file_info":
		payload, err := gateway.sendToPlugin("fileInfo", nil)
		return rawTextResult(payload), err
	case "figma_get_selection":
		depth, err := numberArg(params.Arguments, "depth", 2, 0, 5)
		if err != nil {
			return nil, err
		}
		payload, err := gateway.sendToPlugin("getSelection", map[string]interface{}{"depth": int(depth)})
		return rawTextResult(payload), err
	case "figma_export_selection":
		format, err := stringArg(params.Arguments, "format", "SVG")
		if err != nil {
			return nil, err
		}
		format = strings.ToUpper(format)
		if format != "SVG" && format != "PNG" {
			return nil, errors.New("format must be SVG or PNG")
		}
		scale, err := numberArg(params.Arguments, "scale", 1, 0.1, 4)
		if err != nil {
			return nil, err
		}
		payload, err := gateway.sendToPlugin("exportSelection", map[string]interface{}{"format": format, "scale": scale})
		return rawTextResult(payload), err
	default:
		return nil, fmt.Errorf("unknown tool: %s", params.Name)
	}
}

func tools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "figma_status",
			"title":       "Figma bridge status",
			"description": "Check whether the local Figma plugin bridge is connected.",
			"inputSchema": objectSchema(nil),
		},
		{
			"name":        "figma_file_info",
			"title":       "Figma file info",
			"description": "Get basic information about the open Figma file and current page.",
			"inputSchema": objectSchema(nil),
		},
		{
			"name":        "figma_get_selection",
			"title":       "Get Figma selection",
			"description": "Return a JSON summary of the currently selected Figma nodes.",
			"inputSchema": objectSchema(map[string]interface{}{
				"depth": map[string]interface{}{
					"type":    "integer",
					"minimum": 0,
					"maximum": 5,
					"default": 2,
				},
			}),
		},
		{
			"name":        "figma_export_selection",
			"title":       "Export Figma selection",
			"description": "Export the current Figma selection as SVG or PNG. Result is base64 encoded.",
			"inputSchema": objectSchema(map[string]interface{}{
				"format": map[string]interface{}{
					"type":    "string",
					"enum":    []string{"SVG", "PNG"},
					"default": "SVG",
				},
				"scale": map[string]interface{}{
					"type":    "number",
					"minimum": 0.1,
					"maximum": 4,
					"default": 1,
				},
			}),
		},
	}
}

func objectSchema(properties map[string]interface{}) map[string]interface{} {
	if properties == nil {
		properties = map[string]interface{}{}
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
}

func textResult(value interface{}) map[string]interface{} {
	var text string
	if valueString, ok := value.(string); ok {
		text = valueString
	} else {
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			text = fmt.Sprint(value)
		} else {
			text = string(encoded)
		}
	}

	return map[string]interface{}{
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
	}
}

func rawTextResult(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return textResult(nil)
	}

	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return textResult(string(raw))
	}
	return textResult(value)
}

func numberArg(args map[string]interface{}, key string, fallback, min, max float64) (float64, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return fallback, nil
	}

	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if number < min || number > max {
		return 0, fmt.Errorf("%s must be between %g and %g", key, min, max)
	}
	return number, nil
}

func stringArg(args map[string]interface{}, key, fallback string) (string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return fallback, nil
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return stringValue, nil
}

func writeRPC(writer *bufio.Writer, response rpcResponse) {
	encoded, err := json.Marshal(response)
	if err != nil {
		encoded, _ = json.Marshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      response.ID,
			Error:   &rpcError{Code: -32603, Message: "Internal error"},
		})
	}
	_, _ = writer.Write(encoded)
	_ = writer.WriteByte('\n')
	_ = writer.Flush()
}

func acceptWebSocket(conn net.Conn) (string, error) {
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return "", err
	}

	if !strings.EqualFold(request.Header.Get("Upgrade"), "websocket") {
		return "", errors.New("missing websocket upgrade header")
	}

	key := request.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return "", errors.New("missing Sec-WebSocket-Key header")
	}

	accept := websocketAccept(key)
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"

	_, err = conn.Write([]byte(response))
	if err != nil {
		return "", err
	}
	return request.URL.Path, nil
}

func dialWebSocket(host, port, path string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return nil, err
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = conn.Close()
		return nil, err
	}

	key := base64.StdEncoding.EncodeToString(keyBytes)
	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + net.JoinHostPort(host, port) + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	if _, err := conn.Write([]byte(request)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", response.Status)
	}
	if response.Header.Get("Sec-WebSocket-Accept") != websocketAccept(key) {
		_ = conn.Close()
		return nil, errors.New("websocket upgrade returned an invalid accept key")
	}

	return conn, nil
}

func websocketAccept(key string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	sum := sha1.Sum([]byte(key + magic))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func readWebSocketFrame(conn net.Conn) (byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, nil, err
	}

	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	payloadLength := uint64(header[1] & 0x7f)

	switch payloadLength {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(conn, extended); err != nil {
			return 0, nil, err
		}
		payloadLength = uint64(binary.BigEndian.Uint16(extended))
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(conn, extended); err != nil {
			return 0, nil, err
		}
		payloadLength = binary.BigEndian.Uint64(extended)
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(conn, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, nil, err
	}

	if masked {
		for index := range payload {
			payload[index] ^= maskKey[index%4]
		}
	}

	return opcode, payload, nil
}

func writeWebSocketFrame(conn net.Conn, opcode byte, payload []byte) error {
	header := []byte{0x80 | opcode}

	switch length := len(payload); {
	case length < 126:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127)
		var extended [8]byte
		binary.BigEndian.PutUint64(extended[:], uint64(length))
		header = append(header, extended[:]...)
	}

	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func randomID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func mustRawJSON(value interface{}) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return payload
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func main() {
	log.SetOutput(os.Stderr)

	host := env("FIGMA_BRIDGE_HOST", "localhost")
	port := env("FIGMA_BRIDGE_PORT", "8787")
	if _, err := strconv.Atoi(port); err != nil {
		log.Fatalf("invalid FIGMA_BRIDGE_PORT: %s", port)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if len(os.Args) > 1 && os.Args[1] == "bridge" {
		bridge := newBridge(host, port)
		if err := bridge.listen(ctx); err != nil {
			log.Fatalf("websocket bridge failed: %v", err)
		}
		return
	}

	client := newBridgeClient(host, port)
	if err := serveMCP(ctx, client, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("mcp server failed: %v", err)
	}
}
