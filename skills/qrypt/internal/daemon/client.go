package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync/atomic"

	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

// Client is a JSON-RPC client for qryptd Unix socket.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	reqID  atomic.Int64
}

// DialClient connects to qryptd at the given Unix socket path.
func DialClient(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to qryptd: %w", err)
	}
	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// Close closes the connection.
func (c *Client) Close() {
	c.conn.Close()
}

// Call sends a JSON-RPC request and waits for the response.
func (c *Client) Call(method string, params interface{}) (*protocol.Response, error) {
	id := c.reqID.Add(1)
	req := &protocol.Request{
		ID:     id,
		Method: method,
		Params: params,
	}
	if err := protocol.EncodeRequest(c.conn, req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	resp, err := protocol.DecodeResponse(c.reader)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return resp, nil
}

// SubscribeEvents subscribes to the daemon event stream.
// The returned channel receives events until the connection is closed.
func (c *Client) SubscribeEvents() (<-chan *protocol.Event, error) {
	req := &protocol.Request{
		ID:     c.reqID.Add(1),
		Method: "subscribe_events",
	}
	if err := protocol.EncodeRequest(c.conn, req); err != nil {
		return nil, fmt.Errorf("subscribe events: %w", err)
	}

	// Read acknowledge response
	resp, err := protocol.DecodeResponse(c.reader)
	if err != nil {
		return nil, fmt.Errorf("read subscribe ack: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("subscribe failed: %s", resp.Error.Message)
	}

	ch := make(chan *protocol.Event, 100)
	go func() {
		defer close(ch)
		for {
			r, err := protocol.DecodeRequest(c.reader)
			if err != nil {
				return
			}
			if r == nil || r.Method != "event" {
				continue
			}
			data, err := json.Marshal(r.Params)
			if err != nil {
				continue
			}
			var evt protocol.Event
			if err := json.Unmarshal(data, &evt); err != nil {
				continue
			}
			select {
			case ch <- &evt:
			default:
			}
		}
	}()
	return ch, nil
}

// IsDaemonRunning checks whether qryptd is listening on the default socket.
func IsDaemonRunning(socketPath string) bool {
	if socketPath == "" {
		socketPath = defaultSocketPath()
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// FindSocketPath returns the qryptd socket path from env or default.
func FindSocketPath() string {
	if p := os.Getenv("QRYPTD_SOCKET"); p != "" {
		return p
	}
	return defaultSocketPath()
}

func defaultSocketPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "/tmp/qryptd.sock"
	}
	return home + "/.qrypt/qryptd.sock"
}


