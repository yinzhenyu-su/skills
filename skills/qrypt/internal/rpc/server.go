package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
	"nhooyr.io/websocket"
)

const wsPingInterval = 15 * time.Second

// RPCHost is the interface the daemon exposes to the WebSocket RPC layer.
type RPCHost interface {
	Dashboard() *protocol.DashboardData
	Status() (*protocol.DaemonStatus, error)
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	GetConfig() (*config.Config, error)
	ReloadConfig(ctx context.Context) (*protocol.ReloadResult, error)
	InitConfig(ctx context.Context, path string) (string, error)
	ValidateConfig(path string) (*config.ValidationResult, error)
	SyncStatus() (*protocol.SyncStats, error)
	GetSyncTaskList() ([]protocol.SyncTaskInfo, error)
	CacheUsage() (*protocol.CacheUsage, error)
	ClearStaging(ctx context.Context) error
	ActiveTransfers() *protocol.ActiveTransfersResult
	DaemonShutdown(ctx context.Context) error
	MountList() []protocol.MountSummary
	FileAPI() *qrypt.FileAPI

	SubscribeMountEvents(id string) <-chan *protocol.Event
	UnsubscribeMountEvents(id string)
	SubscribeSyncEvents(id string) <-chan *qrypt.Event
	UnsubscribeSyncEvents(id string)
}

// unmarshalParams re-marshals interface{} params and unmarshals into a typed target.
func unmarshalParams(params interface{}, target interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// WSServer is the WebSocket-based Unix socket server.
type WSServer struct {
	host     RPCHost
	socket   string
	server   *http.Server
	listener net.Listener
	done     chan struct{}
	stopped  chan struct{}
	reqID    atomic.Int64
	headless atomic.Bool

	stopOnce sync.Once

	subsMu    sync.Mutex
	subs      map[string]*websocket.Conn
	connCount atomic.Int32
	idleTimer *time.Timer
	idleMu    sync.Mutex
}

func NewWSServer(host RPCHost, socketPath string) *WSServer {
	return &WSServer{
		host:    host,
		socket:  socketPath,
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
		subs:    make(map[string]*websocket.Conn),
	}
}

func (s *WSServer) SetHeadless(v bool) {
	s.headless.Store(v)
}

func (s *WSServer) Done() <-chan struct{} {
	return s.stopped
}

func (s *WSServer) Start(ctx context.Context) error {
	if err := os.Remove(s.socket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove socket failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.socket), 0700); err != nil {
		return fmt.Errorf("create socket dir failed: %w", err)
	}

	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("listen on socket failed: %w", err)
	}
	s.listener = listener

	if err := os.Chmod(s.socket, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("chmod socket failed: %w", err)
	}

	logging.L.Infof("WS server: listening on %s\n", s.socket)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)

	s.server = &http.Server{Handler: mux}
	go s.server.Serve(listener)

	go s.forwardEvents(ctx)

	return nil
}

func (s *WSServer) Stop() {
	s.stopOnce.Do(func() {
		if s.listener != nil {
			s.listener.Close()
		}
		if s.server != nil {
			s.server.Close()
		}
		close(s.done)
		close(s.stopped)
	})
}

func (s *WSServer) resetIdleTimer() {
	if !s.headless.Load() {
		return
	}
	s.idleMu.Lock()
	defer s.idleMu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	if s.connCount.Load() == 0 {
		const idleTimeout = 30 * time.Second
		s.idleTimer = time.AfterFunc(idleTimeout, func() {
			logging.L.Infof("idle timeout: no connections for %v, shutting down\n", idleTimeout)
			s.host.DaemonShutdown(context.Background())
			s.Stop()
		})
	}
}

func (s *WSServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		logging.L.Errorf("WS accept failed: %v\n", err)
		return
	}

	s.connCount.Add(1)
	s.resetIdleTimer()
	defer func() {
		conn.Close(websocket.StatusNormalClosure, "bye")
		s.connCount.Add(-1)
		s.resetIdleTimer()
	}()

	ctx := r.Context()
	subID := fmt.Sprintf("ws_%d", s.reqID.Add(1))

	pingCtx, pingCancel := context.WithCancel(ctx)
	defer pingCancel()
	go func() {
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.Ping(pingCtx); err != nil {
					return
				}
			case <-pingCtx.Done():
				return
			}
		}
	}()

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			s.subsMu.Lock()
			delete(s.subs, subID)
			s.subsMu.Unlock()
			return
		}

		switch msgType {
		case websocket.MessageText:
			var req protocol.Request
			if err := json.Unmarshal(data, &req); err != nil {
				logging.L.Errorf("WS: invalid JSON: %v\n", err)
				continue
			}

			if req.Method == "subscribe_events" {
				s.subsMu.Lock()
				s.subs[subID] = conn
				s.subsMu.Unlock()

				snapshot := s.host.Dashboard()
				snapshotEvt, _ := json.Marshal(&protocol.Request{
					ID:     0,
					Method: "event",
					Params: &protocol.Event{
						Type:      protocol.EventMountStateChanged,
						Timestamp: time.Now().UnixMilli(),
						Data:      snapshot,
					},
				})
				if err := conn.Write(ctx, websocket.MessageText, snapshotEvt); err != nil {
					logging.L.Errorf("WS: write snapshot failed: %v\n", err)
				}

				ack, _ := json.Marshal(protocol.NewResult(req.ID, map[string]string{"status": "subscribed"}))
				if err := conn.Write(ctx, websocket.MessageText, ack); err != nil {
					logging.L.Errorf("WS: write ack failed: %v\n", err)
				}
				continue
			}

			if req.Method == "cat_file" {
				s.streamCatFile(ctx, conn, &req)
				continue
			}

			resp := s.dispatch(ctx, &req)
			respData, err := json.Marshal(resp)
			if err != nil {
				logging.L.Errorf("WS: marshal response failed: %v\n", err)
				continue
			}
			if err := conn.Write(ctx, websocket.MessageText, respData); err != nil {
				logging.L.Errorf("WS: write response failed: %v\n", err)
				return
			}

		case websocket.MessageBinary:
			logging.L.Debugf("WS: ignoring binary frame (%d bytes)\n", len(data))
		}
	}
}

func (s *WSServer) forwardEvents(ctx context.Context) {
	mountCh := s.host.SubscribeMountEvents("ws_broadcast")
	defer s.host.UnsubscribeMountEvents("ws_broadcast")

	syncCh := s.host.SubscribeSyncEvents("ws_sync")
	defer s.host.UnsubscribeSyncEvents("ws_sync")

	for {
		select {
		case evt, ok := <-mountCh:
			if !ok {
				return
			}
			s.broadcast(ctx, evt)
		case evt, ok := <-syncCh:
			if !ok {
				return
			}
			s.broadcastSyncEvent(ctx, evt)
		case <-s.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *WSServer) broadcast(ctx context.Context, evt *protocol.Event) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for id, conn := range s.subs {
		data, _ := json.Marshal(&protocol.Request{
			ID:     0,
			Method: "event",
			Params: evt,
		})
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			delete(s.subs, id)
		}
	}
}

func (s *WSServer) broadcastSyncEvent(ctx context.Context, evt *qrypt.Event) {
	protoEvt := &protocol.Event{
		Type:      protocol.EventType(evt.Type),
		Mount:     evt.Mount,
		Timestamp: evt.Timestamp,
		Data:      evt.Progress,
	}
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for id, conn := range s.subs {
		data, _ := json.Marshal(&protocol.Request{
			ID:     0,
			Method: "event",
			Params: protoEvt,
		})
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			delete(s.subs, id)
		}
	}
}

func (s *WSServer) streamCatFile(ctx context.Context, conn *websocket.Conn, req *protocol.Request) {
	var params struct {
		MountName string `json:"mount_name"`
		Path      string `json:"path"`
	}
	if req.Params != nil {
		if err := unmarshalParams(req.Params, &params); err != nil {
			errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error()))
			conn.Write(ctx, websocket.MessageText, errResp)
			return
		}
	}

	api := s.host.FileAPI()
	if api == nil {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, "FileAPI not available"))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}

	rc, err := api.Read(ctx, params.MountName, params.Path)
	if err != nil {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, fmt.Sprintf("read: %v", err)))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}
	defer rc.Close()

	buf := make([]byte, 262144)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			if werr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
				return
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
	}

	eof, _ := json.Marshal(protocol.NewResult(req.ID, map[string]bool{"eof": true}))
	conn.Write(ctx, websocket.MessageText, eof)
}

func (s *WSServer) dispatch(ctx context.Context, req *protocol.Request) *protocol.Response {
	id := req.ID

	switch req.Method {
	case "ping":
		return protocol.NewResult(id, map[string]string{"status": "pong"})

	case "dashboard":
		return protocol.NewResult(id, s.host.Dashboard())

	case "status":
		status, err := s.host.Status()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, err.Error())
		}
		return protocol.NewResult(id, status)

	case "start":
		var p struct{ Name string `json:"name,omitempty"` }
		if req.Params != nil {
			unmarshalParams(req.Params, &p)
		}
		go s.host.Start(ctx, p.Name)
		return protocol.NewResult(id, map[string]string{"status": "accepted"})

	case "stop":
		var p struct{ Name string `json:"name,omitempty"` }
		if req.Params != nil {
			unmarshalParams(req.Params, &p)
		}
		go s.host.Stop(ctx, p.Name)
		return protocol.NewResult(id, map[string]string{"status": "accepted"})

	case "mount_list":
		return protocol.NewResult(id, s.host.MountList())

	case "get_config":
		cfg, err := s.host.GetConfig()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeConfig, err.Error())
		}
		return protocol.NewResult(id, cfg)

	case "reload_config":
		result, err := s.host.ReloadConfig(ctx)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeBusy, err.Error())
		}
		return protocol.NewResult(id, result)

	case "init_config":
		var params struct {
			Path string `json:"path,omitempty"`
		}
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &params); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		created, err := s.host.InitConfig(ctx, params.Path)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeConfig, err.Error())
		}
		return protocol.NewResult(id, map[string]string{"status": "created", "path": created})

	case "validate_config":
		var vparams struct {
			Path string `json:"path,omitempty"`
		}
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &vparams); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		result, err := s.host.ValidateConfig(vparams.Path)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeConfig, err.Error())
		}
		return protocol.NewResult(id, result)

	case "sync_status":
		stats, err := s.host.SyncStatus()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, stats)

	case "sync_task_list":
		tasks, err := s.host.GetSyncTaskList()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, tasks)

	case "cache_usage":
		usage, err := s.host.CacheUsage()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeCache, err.Error())
		}
		return protocol.NewResult(id, usage)

	case "clear_staging":
		err := s.host.ClearStaging(ctx)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeCache, err.Error())
		}
		return s.handleAction(id, err, "cleared")

	case "push_start":
		var p struct {
			MountName string `json:"mount_name"`
			Source    string `json:"source"`
			Remote    string `json:"remote"`
		}
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		api := s.host.FileAPI()
		if api == nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, "FileAPI not available")
		}
		if err := api.Push(ctx, p.MountName, p.Source, p.Remote); err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, map[string]string{"status": "accepted"})

	case "pull_start":
		var p struct {
			MountName string `json:"mount_name"`
			Remote    string `json:"remote"`
			Local     string `json:"local"`
		}
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		api := s.host.FileAPI()
		if api == nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, "FileAPI not available")
		}
		if err := api.Pull(ctx, p.MountName, p.Remote, p.Local); err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, map[string]string{"status": "accepted"})

	case "list_dir":
		var p protocol.ListDirParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		api := s.host.FileAPI()
		if api == nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, "FileAPI not available")
		}
		entries, err := api.List(ctx, p.MountName, p.Path)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		result := protocol.ListDirResult{Path: p.Path}
		for _, e := range entries {
			result.Entries = append(result.Entries, protocol.ListEntryItem{
				ID: e.ID, Name: e.Name, DecName: e.DecName,
				IsDir: e.IsDir, Size: e.Size, PlainSize: e.PlainSize,
				ModTime: e.ModTime.UnixMilli(),
			})
		}
		return protocol.NewResult(id, result)

	case "mkdir":
		var p protocol.MkdirParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		api := s.host.FileAPI()
		if api == nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, "FileAPI not available")
		}
		if err := api.Mkdir(ctx, p.MountName, p.Path); err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, map[string]string{"status": "created"})

	case "remove":
		var p protocol.RemoveParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		api := s.host.FileAPI()
		if api == nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, "FileAPI not available")
		}
		if err := api.Remove(ctx, p.MountName, p.Path, p.Recursive); err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, map[string]string{"status": "removed"})

	case "move":
		var p protocol.MoveParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		api := s.host.FileAPI()
		if api == nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, "FileAPI not available")
		}
		if err := api.Move(ctx, p.MountName, p.SrcPath, p.DstPath); err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, map[string]string{"status": "moved"})

	case "find":
		var p protocol.FindParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		api := s.host.FileAPI()
		if api == nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, "FileAPI not available")
		}
		entries, err := api.Find(ctx, p.MountName, p.Path, p.Pattern, p.MaxDepth, p.MaxMatches, p.CaseSensitive)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		result := protocol.FindResult{}
		for _, e := range entries {
			result.Entries = append(result.Entries, protocol.FindEntry{
				Path: e.DecName, IsDir: e.IsDir, Size: e.Size,
			})
		}
		result.Count = len(result.Entries)
		return protocol.NewResult(id, result)

	case "active_transfers":
		return protocol.NewResult(id, s.host.ActiveTransfers())

	case "shutdown":
		go func() {
			s.host.DaemonShutdown(ctx)
			s.Stop()
		}()
		return protocol.NewResult(id, map[string]string{"status": "shutdown"})

	default:
		return protocol.NewError(id, protocol.ErrCodeMethodNotFound,
			fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *WSServer) handleAction(id int64, err error, okMsg string) *protocol.Response {
	if err != nil {
		code := deriveErrorCode(err)
		return protocol.NewError(id, code, err.Error())
	}
	return protocol.NewResult(id, map[string]string{"status": okMsg})
}

func deriveErrorCode(err error) int {
	msg := err.Error()
	switch {
	case containsAny(msg, "config", "配置"):
		return protocol.ErrCodeConfig
	case containsAny(msg, "mount", "挂载", "not found in config", "already running", "not running"):
		return protocol.ErrCodeMount
	case containsAny(msg, "sync", "upload", "下载"):
		return protocol.ErrCodeSync
	case containsAny(msg, "cache"):
		return protocol.ErrCodeCache
	case containsAny(msg, "busy", "pending"):
		return protocol.ErrCodeBusy
	case containsAny(msg, "invalid", "参数"):
		return protocol.ErrCodeInvalidReq
	default:
		return protocol.ErrCodeInternal
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
