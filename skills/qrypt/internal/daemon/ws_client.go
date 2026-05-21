package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
	"nhooyr.io/websocket"
)

// WSClient is a WebSocket-based RPC client for qryptd.
// Single connection handles RPC calls, event subscription, and binary data.
type WSClient struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	pending map[int64]chan *protocol.Response
	nextID  atomic.Int64

	eventCh chan *protocol.Event
	wg      sync.WaitGroup
}

// DialWS connects to qryptd at the given Unix socket path via WebSocket.
func DialWS(socketPath string) (*WSClient, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	httpClient := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())

	conn, _, err := websocket.Dial(ctx, "http://unix/ws", &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect to qryptd: %w", err)
	}

	c := &WSClient{
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
		pending: make(map[int64]chan *protocol.Response),
		eventCh: make(chan *protocol.Event, 100),
	}

	c.wg.Add(1)
	go c.readLoop()

	return c, nil
}

// Conn returns the underlying WebSocket connection (for binary streaming).
func (c *WSClient) Conn() *websocket.Conn { return c.conn }

// Ctx returns the client's context.
func (c *WSClient) Ctx() context.Context { return c.ctx }

// Close closes the connection.
func (c *WSClient) Close() {
	c.cancel()
	c.conn.Close(websocket.StatusNormalClosure, "bye")
	c.wg.Wait()
}

// Call sends an RPC request and waits for the response.
func (c *WSClient) Call(method string, params interface{}) (*protocol.Response, error) {
	id := c.nextID.Add(1)
	req := &protocol.Request{
		ID:     id,
		Method: method,
		Params: params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ch := make(chan *protocol.Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.conn.Write(c.ctx, websocket.MessageText, data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("send request: %w", err)
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
}

// Events returns a channel that receives server-pushed events.
func (c *WSClient) Events() <-chan *protocol.Event {
	return c.eventCh
}

// SubscribeEvents subscribes to the daemon event stream and returns the event channel.
func (c *WSClient) SubscribeEvents() (<-chan *protocol.Event, error) {
	resp, err := c.Call("subscribe_events", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("subscribe failed: %s", resp.Error.Message)
	}
	return c.eventCh, nil
}

func (c *WSClient) readLoop() {
	defer c.wg.Done()

	for {
		msgType, data, err := c.conn.Read(c.ctx)
		if err != nil {
			return
		}

		switch msgType {
		case websocket.MessageText:
			// Try as Response (has id > 0)
			var resp protocol.Response
			if err := json.Unmarshal(data, &resp); err == nil && resp.ID > 0 {
				c.mu.Lock()
				ch, ok := c.pending[resp.ID]
				delete(c.pending, resp.ID)
				c.mu.Unlock()
				if ok {
					select {
					case ch <- &resp:
					default:
					}
				}
				continue
			}

			// Try as Event (id == 0, method == "event")
			var req protocol.Request
			if err := json.Unmarshal(data, &req); err == nil && req.ID == 0 && req.Method == "event" {
				evtData, _ := json.Marshal(req.Params)
				var evt protocol.Event
				if json.Unmarshal(evtData, &evt) == nil {
					select {
					case c.eventCh <- &evt:
					default:
					}
				}
				continue
			}

		case websocket.MessageBinary:
			// Future: file data streaming
		}
	}
}
