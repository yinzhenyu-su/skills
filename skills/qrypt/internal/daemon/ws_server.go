package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
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
	stopped  chan struct{}
	reqID    atomic.Int64
	headless atomic.Bool

	stopOnce sync.Once

	subsMu     sync.Mutex
	subs       map[string]*websocket.Conn
	connCount  atomic.Int32
	idleTimer  *time.Timer
	idleMu     sync.Mutex
}

// SetHeadless marks the server as headless (no FUSE mount).
// In headless mode, the server auto-exits after an idle timeout.
func (s *WSServer) SetHeadless(v bool) {
	s.headless.Store(v)
}

// Done returns a channel closed when the server has stopped.
func (s *WSServer) Done() <-chan struct{} {
	return s.stopped
}

func NewWSServer(daemon *Daemon, socketPath string) *WSServer {
	return &WSServer{
		daemon:  daemon,
		socket:  socketPath,
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
		subs:    make(map[string]*websocket.Conn),
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

// resetIdleTimer resets the idle shutdown timer for headless mode.
// When no connections are active in headless mode, the server auto-exits after a timeout.
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
			log.L.Infof("idle timeout: no connections for %v, shutting down\n", idleTimeout)
			s.daemon.DaemonShutdown(context.Background())
			s.Stop()
		})
	}
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

func (s *WSServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.L.Errorf("WS accept failed: %v\n", err)
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

			// Binary streaming methods bypass normal dispatch.
			if req.Method == "cat_file" {
				s.streamCatFile(ctx, conn, &req)
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

// streamCatFile reads a remote file, decrypts it, and streams the plaintext
// as binary frames over the WebSocket connection. Finishes with a text EOF response.
func (s *WSServer) streamCatFile(ctx context.Context, conn *websocket.Conn, req *protocol.Request) {
	var params struct {
		MountName string `json:"mount_name"`
		Path      string `json:"path"`
		Password  string `json:"password"`
		Salt      string `json:"salt"`
	}
	if req.Params != nil {
		if err := unmarshalParams(req.Params, &params); err != nil {
			errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error()))
			conn.Write(ctx, websocket.MessageText, errResp)
			return
		}
	}

	name := s.daemon.resolveMountName(params.MountName)
	mountCfg := config.FindMount(s.daemon.cfg, name)
	if mountCfg == nil {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeMount, "mount not found"))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}
	rc := s.daemon.cfg.MergeInstanceConfig(*mountCfg)

	cipher, err := s.daemon.makeCipher(rc, params.Password, params.Salt)
	if err != nil {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, err.Error()))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}

	sk, _ := SessionKeyForMount(rc)
	ses, err := s.daemon.sessionMgr.Acquire(ctx, sk, rc.Params)
	if err != nil {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, "session: "+err.Error()))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}
	defer s.daemon.sessionMgr.Release(ctx, sk)

	drv := ses.Drv
	resolver, ok := drv.(drive.PathResolver)
	if !ok {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, "driver does not support path resolution"))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}

	rootPath := config.RootPathForMount(*mountCfg)
	fullPath := config.ResolveFullPath(rootPath, params.Path)

	parentPath := s.daemon.dirOf(fullPath)
	baseName := baseOf(fullPath)

	parentFid, err := resolver.ResolvePath(ctx, parentPath)
	if err != nil {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, fmt.Sprintf("resolve path: %v", err)))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}

	entries, err := drv.List(ctx, parentFid)
	if err != nil {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, fmt.Sprintf("list: %v", err)))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}

	encName := cipher.EncryptSegment(baseName)
	var targetEntry drive.Entry
	found := false
	for _, e := range entries {
		if e.Name == encName {
			targetEntry = e
			found = true
			break
		}
	}
	if !found {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, "file not found"))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}
	if targetEntry.IsDir {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, "is a directory"))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}

	encSize := targetEntry.Size
	headerSize := int64(crypt.FileHeaderSize)
	if encSize <= headerSize {
		// Empty file, send EOF directly
		conn.Write(ctx, websocket.MessageText, []byte(`{"id":`+strconv.FormatInt(req.ID, 10)+`,"result":{"eof":true}}`))
		return
	}

	header, err := drv.Read(ctx, targetEntry, 0, headerSize)
	if err != nil {
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, fmt.Sprintf("read header: %v", err)))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}
	headerBytes := make([]byte, headerSize)
	if _, err := io.ReadFull(header, headerBytes); err != nil {
		header.Close()
		errResp, _ := json.Marshal(protocol.NewError(req.ID, protocol.ErrCodeInternal, fmt.Sprintf("read header: %v", err)))
		conn.Write(ctx, websocket.MessageText, errResp)
		return
	}
	header.Close()

	var fileNonce [crypt.FileNonceSize]byte
	copy(fileNonce[:], headerBytes[crypt.FileMagicSize:])

	bodySize := encSize - headerSize
	const blocksPerChunk = 64
	chunkEncBytes := int64(blocksPerChunk * crypt.BlockSize)
	numChunks := int((bodySize + chunkEncBytes - 1) / chunkEncBytes)

	for chunk := range numChunks {
		off := headerSize + int64(chunk)*chunkEncBytes
		sz := chunkEncBytes
		if chunk == numChunks-1 {
			sz = bodySize - int64(chunk)*chunkEncBytes
		}

		rc, err := drv.Read(ctx, targetEntry, off, sz)
		if err != nil {
			break
		}
		encData := make([]byte, sz)
		if _, err := io.ReadFull(rc, encData); err != nil {
			rc.Close()
			break
		}
		rc.Close()

		baseBlock := chunk * blocksPerChunk
		pos := 0
		for pos < len(encData) {
			blockEnd := pos + crypt.BlockSize
			if blockEnd > len(encData) {
				blockEnd = len(encData)
			}
			plain, err := cipher.DecryptBlock(encData[pos:blockEnd], uint64(baseBlock+pos/crypt.BlockSize), fileNonce)
			if err != nil {
				conn.Write(ctx, websocket.MessageText, []byte(`{"id":`+strconv.FormatInt(req.ID, 10)+`,"error":{"code":-1,"message":"decrypt failed"}}`))
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, plain); err != nil {
				return
			}
			pos = blockEnd
		}
	}

	// Send EOF
	eof, _ := json.Marshal(protocol.NewResult(req.ID, map[string]bool{"eof": true}))
	conn.Write(ctx, websocket.MessageText, eof)
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

	case "find":
		var p protocol.FindParams
		if req.Params != nil {
			if err := unmarshalParams(req.Params, &p); err != nil {
				return protocol.NewError(id, protocol.ErrCodeInvalidReq, "invalid params: "+err.Error())
			}
		}
		result, err := s.daemon.Find(ctx, p)
		if err != nil {
			return protocol.NewError(id, protocol.ErrCodeSync, err.Error())
		}
		return protocol.NewResult(id, result)

	case "active_transfers":
		return protocol.NewResult(id, s.daemon.ActiveTransfers())

	case "shutdown":
		go func() {
			s.daemon.DaemonShutdown(ctx)
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
		return protocol.NewError(id, protocol.ErrCodeInternal, err.Error())
	}
	return protocol.NewResult(id, map[string]string{"status": okMsg})
}
