// Package lspclient implements the bounded stdio JSON-RPC lifecycle needed by
// focused language-server experiments. It intentionally does not model LSP
// methods; language adapters own their request and response types.
package lspclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const maxMessageBytes = 16 * 1024 * 1024

type Options struct {
	Binary        string
	Args          []string
	Dir           string
	Configuration map[string]any
}

type Client struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	reader   *bufio.Reader
	messages chan responseEnvelope
	readErr  chan error
	config   map[string]any

	writeMu sync.Mutex
	nextID  int64
	waited  bool
	closed  bool
}

type responseEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func Start(ctx context.Context, opts Options) (*Client, error) {
	if strings.TrimSpace(opts.Binary) == "" {
		return nil, fmt.Errorf("lspclient: binary is required")
	}
	cmd := exec.CommandContext(ctx, opts.Binary, opts.Args...)
	cmd.Dir = opts.Dir
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lspclient: open stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lspclient: open stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lspclient: start %q: %w", opts.Binary, err)
	}
	client := &Client{
		cmd:      cmd,
		stdin:    stdin,
		reader:   bufio.NewReader(stdout),
		messages: make(chan responseEnvelope, 32),
		readErr:  make(chan error, 1),
		config:   opts.Configuration,
		nextID:   1,
	}
	go client.readLoop()
	return client, nil
}

func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	if strings.TrimSpace(method) == "" {
		return fmt.Errorf("lspclient: method is required")
	}
	id := c.nextID
	c.nextID++
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if err := c.write(request); err != nil {
		return fmt.Errorf("lspclient: send %s: %w", method, err)
	}

	for {
		var message responseEnvelope
		select {
		case <-ctx.Done():
			return fmt.Errorf("lspclient: %s: %w", method, ctx.Err())
		case err := <-c.readErr:
			return fmt.Errorf("lspclient: receive %s: %w", method, err)
		case message = <-c.messages:
		}
		if message.Method != "" && len(message.ID) > 0 {
			if err := c.respondToServerRequest(message); err != nil {
				return fmt.Errorf("lspclient: respond to %s: %w", message.Method, err)
			}
			continue
		}
		if !sameID(message.ID, id) {
			continue
		}
		if message.Error != nil {
			return fmt.Errorf("lspclient: %s: server error %d: %s", method, message.Error.Code, message.Error.Message)
		}
		if result == nil || len(message.Result) == 0 || string(message.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(message.Result, result); err != nil {
			return fmt.Errorf("lspclient: decode %s result: %w", method, err)
		}
		return nil
	}
}

func (c *Client) Notify(method string, params any) error {
	if strings.TrimSpace(method) == "" {
		return fmt.Errorf("lspclient: method is required")
	}
	return c.write(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func (c *Client) Close(ctx context.Context) error {
	if c.closed {
		return nil
	}
	c.closed = true

	var closeErr error
	if c.cmd.ProcessState == nil {
		if err := c.Call(ctx, "shutdown", nil, nil); err != nil {
			closeErr = err
			if killErr := c.cmd.Process.Kill(); killErr != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("lspclient: kill after failed shutdown: %w", killErr))
			}
		}
		if closeErr == nil {
			if err := c.Notify("exit", nil); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("lspclient: send exit: %w", err))
			}
		}
	}
	if err := c.stdin.Close(); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("lspclient: close stdin: %w", err))
	}
	if !c.waited {
		c.waited = true
		if err := c.cmd.Wait(); err != nil && closeErr == nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("lspclient: wait: %w", err))
		}
	}
	return closeErr
}

func (c *Client) write(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if len(payload) > maxMessageBytes {
		return fmt.Errorf("message exceeds %d bytes", maxMessageBytes)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	if _, err := c.stdin.Write(payload); err != nil {
		return err
	}
	return nil
}

func (c *Client) read() (responseEnvelope, error) {
	contentLength := -1
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return responseEnvelope{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		contentLength, err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return responseEnvelope{}, fmt.Errorf("invalid content length: %w", err)
		}
	}
	if contentLength < 0 || contentLength > maxMessageBytes {
		return responseEnvelope{}, fmt.Errorf("invalid content length %d", contentLength)
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return responseEnvelope{}, err
	}
	var message responseEnvelope
	if err := json.Unmarshal(payload, &message); err != nil {
		return responseEnvelope{}, fmt.Errorf("decode message: %w", err)
	}
	return message, nil
}

func (c *Client) readLoop() {
	for {
		message, err := c.read()
		if err != nil {
			c.readErr <- err
			return
		}
		// Notifications never satisfy a request and can be noisy while Pyright
		// indexes a workspace. Drop them before the bounded response queue.
		if message.Method != "" && len(message.ID) == 0 {
			continue
		}
		c.messages <- message
	}
}

func (c *Client) respondToServerRequest(message responseEnvelope) error {
	var result any
	if message.Method == "workspace/configuration" {
		var params struct {
			Items []struct {
				Section string `json:"section"`
			} `json:"items"`
		}
		if err := json.Unmarshal(message.Params, &params); err == nil {
			values := make([]any, 0, len(params.Items))
			for _, item := range params.Items {
				values = append(values, nestedConfiguration(c.config, item.Section))
			}
			result = values
		}
	}
	return c.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      message.ID,
		"result":  result,
	})
}

func nestedConfiguration(config map[string]any, section string) any {
	var current any = config
	for _, part := range strings.Split(section, ".") {
		values, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = values[part]
		if !ok {
			return nil
		}
	}
	return current
}

func sameID(raw json.RawMessage, id int64) bool {
	if len(raw) == 0 {
		return false
	}
	var got int64
	return json.Unmarshal(raw, &got) == nil && got == id
}
