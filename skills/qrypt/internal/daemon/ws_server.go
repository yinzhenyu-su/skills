package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
	"nhooyr.io/websocket"
)

// WSServer is the WebSocket-based Unix socket server.
type WSServer struct {
	daemon   *Daemon
	socket   string
	server   *http.Server
	listener net.Listener
	done     chan struct{}
	reqID    atomic.Int64

	subsMu sync.Mutex
	subs   map[string]*websocket.Conn
}

func NewWSServer(daemon *Daemon, socketPath string) *WSServer {
	return &WSServer{
		daemon: daemon,
		socket: socketPath,
		done:   make(chan struct{}),
		subs:   make(map[string]*websocket.Conn),
	}
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

	log.L.Infof("WS server: listening on %s\n", s.socket)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)

	s.server = &http.Server{Handler: mux}
	go s.server.Serve(listener)

	// Forward daemon events to all WS subscribers.
	go s.forwardEvents(ctx)

	return nil
}

func (s *WSServer) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
	if s.server != nil {
		s.server.Close()
	}
	close(s.done)
}

func (s *WSServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.L.Errorf("WS accept failed: %v\n", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "bye")

	ctx := r.Context()
	subID := fmt.Sprintf("ws_%d", s.reqID.Add(1))

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
				log.L.Errorf("WS: invalid JSON: %v\n", err)
				continue
			}

			if req.Method == "subscribe_events" {
				s.subsMu.Lock()
				s.subs[subID] = conn
				s.subsMu.Unlock()

				ack, _ := json.Marshal(protocol.NewResult(req.ID, map[string]string{"status": "subscribed"}))
				if err := conn.Write(ctx, websocket.MessageText, ack); err != nil {
					log.L.Errorf("WS: write ack failed: %v\n", err)
				}
				continue
			}

			resp := s.dispatch(ctx, &req)
			respData, err := json.Marshal(resp)
			if err != nil {
				log.L.Errorf("WS: marshal response failed: %v\n", err)
				continue
			}
			if err := conn.Write(ctx, websocket.MessageText, respData); err != nil {
				log.L.Errorf("WS: write response failed: %v\n", err)
				return
			}

		case websocket.MessageBinary:
			log.L.Debugf("WS: ignoring binary frame (%d bytes)\n", len(data))
		}
	}
}

func (s *WSServer) forwardEvents(ctx context.Context) {
	ch := s.daemon.eventMgr.Subscribe("ws_broadcast")
	defer s.daemon.eventMgr.Unsubscribe("ws_broadcast")

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			s.subsMu.Lock()
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
			s.subsMu.Unlock()
		case <-s.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *WSServer) dispatch(ctx context.Context, req *protocol.Request) *protocol.Response {
	id := req.ID

	switch req.Method {
	case "status":
		status, err := s.daemon.Status()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, err.Error())
		}
		return protocol.NewResult(id, status)

	case "start":
		var p struct{ Name string `json:"name,omitempty"` }
		if req.Params != nil {
			unmarshalParams(req.Params, &p)
		}
		err := s.daemon.Start(ctx, p.Name)
		return s.handleAction(id, err, "started")

	case "stop":
		var p struct{ Name string `json:"name,omitempty"` }
		if req.Params != nil {
			unmarshalParams(req.Params, &p)
		}
		err := s.daemon.Stop(ctx, p.Name)
		return s.handleAction(id, err, "stopped")

	case "mount_list":
		return protocol.NewResult(id, s.daemon.manager.List())

	case "get_config":
		cfg, err := s.daemon.GetConfig()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeConfig, err.Error())
		}
		return protocol.NewResult(id, cfg)

	case "reload_config":
		result, err := s.daemon.ReloadConfig(ctx)
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
		created, err := s.daemon.InitConfig(ctx, params.Path)
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
		result, err := s.daemon.ValidateConfig(vparams.Path)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeConfig, err.Error())
		}
		return protocol.NewResult(id, result)

	case "sync_status":
		stats, err := s.daemon.SyncStatus()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, stats)

	case "sync_task_list":
		tasks, err := s.daemon.GetSyncTaskList()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, tasks)

	case "cache_usage":
		usage, err := s.daemon.CacheUsage()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeCache, err.Error())
		}
		return protocol.NewResult(id, usage)

	case "clear_staging":
		err := s.daemon.ClearStaging(ctx)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeCache, err.Error())
		}
		return s.handleAction(id, err, "cleared")

	case "push_start":
		var p protocol.PushStartParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		result, err := s.daemon.PushStart(ctx, p)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, result)

	case "pull_start":
		var p protocol.PullStartParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		result, err := s.daemon.PullStart(ctx, p)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, result)

	case "list_dir":
		var p protocol.ListDirParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		result, err := s.daemon.ListDir(ctx, p)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, result)

	case "mkdir":
		var p protocol.MkdirParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		result, err := s.daemon.Mkdir(ctx, p)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, result)

	case "remove":
		var p protocol.RemoveParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		result, err := s.daemon.Remove(ctx, p)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, result)

	case "move":
		var p protocol.MoveParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		result, err := s.daemon.Move(ctx, p)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, result)

	default:
		return protocol.NewError(id, protocol.ErrCodeMethodNotFound,
			fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *WSServer) handleAction(id int64, err error, okMsg string) *protocol.Response {
	if err != nil {
		return protocol.NewError(id, protocol.ErrCodeInternal, err.Error())
	}
	return protocol.NewResult(id, map[string]string{"status": okMsg})
}
