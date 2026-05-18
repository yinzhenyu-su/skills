package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

// Server is the Unix socket JSON-RPC server.
type Server struct {
	daemon  *Daemon
	socket  string
	listener net.Listener
	done     chan struct{}
	reqID   atomic.Int64
}

// NewServer creates a new JSON-RPC server.
func NewServer(daemon *Daemon, socketPath string) *Server {
	return &Server{
		daemon: daemon,
		socket: socketPath,
		done:   make(chan struct{}),
	}
}

// Start starts the Unix socket listener.
func (s *Server) Start(ctx context.Context) error {
	// Remove existing socket file
	if err := os.Remove(s.socket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove socket failed: %w", err)
	}

	// Ensure parent dir exists
	if err := os.MkdirAll(filepath.Dir(s.socket), 0700); err != nil {
		return fmt.Errorf("create socket dir failed: %w", err)
	}

	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("listen on socket failed: %w", err)
	}
	s.listener = listener

	// Set permissions so only owner can access
	if err := os.Chmod(s.socket, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("chmod socket failed: %w", err)
	}

	log.L.Infof("Daemon server: listening on %s\n", s.socket)

	go s.acceptLoop(ctx)
	return nil
}

// Stop stops the server.
func (s *Server) Stop() {
	// Close listener first to unblock Accept(), then signal done
	if s.listener != nil {
		s.listener.Close()
	}
	close(s.done)
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// listener closed → s.done is about to be closed; exit
			return
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	log.L.Debugf("Daemon server: new connection from %s\n", conn.RemoteAddr())

	reader := bufio.NewReader(conn)

	for {
		req, err := protocol.DecodeRequest(reader)
		if err != nil {
			// Connection closed or error
			return
		}
		if req == nil {
			continue
		}

		// Handle event subscription
		if req.Method == "subscribe_events" {
			s.handleEventStream(ctx, conn, req)
			return
		}

		resp := s.dispatch(ctx, req)
		if err := protocol.EncodeResponse(conn, resp); err != nil {
			log.L.Errorf("Daemon server: write response failed: %v\n", err)
			return
		}
	}
}

func (s *Server) handleEventStream(ctx context.Context, conn net.Conn, req *protocol.Request) {
	subID := fmt.Sprintf("conn_%d", s.reqID.Add(1))
	ch := s.daemon.eventMgr.Subscribe(subID)
	defer s.daemon.eventMgr.Unsubscribe(subID)

	// Send initial acknowledge
	ack := protocol.NewResult(req.ID, map[string]string{"status": "subscribed"})
	if err := protocol.EncodeResponse(conn, ack); err != nil {
		return
	}

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if err := protocol.EncodeEvent(conn, evt); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) dispatch(ctx context.Context, req *protocol.Request) *protocol.Response {
	id := req.ID

	switch req.Method {
	case "status":
		status, err := s.daemon.Status()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeInternal, err.Error())
		}
		return protocol.NewResult(id, status)

	case "start":
		err := s.daemon.Start(ctx)
		return s.handleAction(id, err, "started")

	case "stop":
		err := s.daemon.Stop(ctx)
		return s.handleAction(id, err, "stopped")

	case "mount_status":
		state, err := s.daemon.MountStatus()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeMount, err.Error())
		}
		return protocol.NewResult(id, state)

	case "get_config":
		cfg, err := s.daemon.GetConfig()
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeConfig, err.Error())
		}
		return protocol.NewResult(id, cfg)

	case "update_config":
		var patch protocol.ConfigPatch
		if err := unmarshalParams(req.Params, &patch); err != nil {
			return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
		}
		if err := s.daemon.UpdateConfig(ctx, patch); err != nil {
			return protocol.NewError(id, protocol.ErrCodeConfig, err.Error())
		}
		return protocol.NewResult(id, map[string]string{"status": "updated"})

	case "validate_config":
		var patch protocol.ConfigPatch
		if err := unmarshalParams(req.Params, &patch); err != nil {
			return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
		}
		if err := s.daemon.ValidateConfig(patch); err != nil {
			return protocol.NewResult(id, map[string]string{"valid": "false", "error": err.Error()})
		}
		return protocol.NewResult(id, map[string]string{"valid": "true"})

	case "login_cookie":
		var p struct{ Cookie string `json:"cookie"` }
		if err := unmarshalParams(req.Params, &p); err != nil {
			return protocol.NewError(id, protocol.ErrCodeInvalidReq, err.Error())
		}
		if err := s.daemon.LoginCookie(ctx, p.Cookie); err != nil {
			return protocol.NewError(id, protocol.ErrCodeAuth, err.Error())
		}
		return protocol.NewResult(id, map[string]string{"status": "logged_in"})

	case "login_qr":
		url, expires, err := s.daemon.LoginQR(ctx)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeAuth, err.Error())
		}
		return protocol.NewResult(id, map[string]interface{}{
			"qr_url":   url,
			"expires_at": expires,
		})

	case "logout":
		err := s.daemon.Logout(ctx)
		return s.handleAction(id, err, "logged_out")

	case "is_logged_in":
		return protocol.NewResult(id, s.daemon.IsLoggedIn())

	case "account_info":
		info, err := s.daemon.GetAccountInfo(ctx)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeAuth, err.Error())
		}
		return protocol.NewResult(id, info)

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

	default:
		return protocol.NewError(id, protocol.ErrCodeMethodNotFound,
			fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleResult(id int64, result interface{}, err error) *protocol.Response {
	if err != nil {
		return protocol.NewError(id, protocol.ErrCodeInternal, err.Error())
	}
	return protocol.NewResult(id, result)
}

func (s *Server) handleAction(id int64, err error, okMsg string) *protocol.Response {
	if err != nil {
		return protocol.NewError(id, protocol.ErrCodeInternal, err.Error())
	}
	return protocol.NewResult(id, map[string]string{"status": okMsg})
}

func unmarshalParams(params interface{}, target interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
