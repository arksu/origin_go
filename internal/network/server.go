package network

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/types"
)

type Server struct {
	cfg        *config.NetworkConfig
	listener   net.Listener
	httpServer *http.Server
	logger     *zap.Logger

	clients   map[uint64]*Client
	clientsMu sync.RWMutex
	nextID    atomic.Uint64

	onConnect    func(*Client)
	onDisconnect func(*Client)
	onMessage    func(*Client, []byte)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type Client struct {
	ID          uint64
	conn        net.Conn
	server      *Server
	logger      *zap.Logger
	sendCh      chan []byte
	closeCh     chan struct{}
	closeOnce   sync.Once
	CharacterID types.EntityID
	Layer       int
}

func NewServer(cfg *config.NetworkConfig, logger *zap.Logger) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		cfg:     cfg,
		logger:  logger,
		clients: make(map[uint64]*Client),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (s *Server) SetOnConnect(fn func(*Client)) {
	s.onConnect = fn
}

func (s *Server) SetOnDisconnect(fn func(*Client)) {
	s.onDisconnect = fn
}

func (s *Server) SetOnMessage(fn func(*Client, []byte)) {
	s.onMessage = fn
}

func (s *Server) Start(addr string, mux *http.ServeMux) error {
	if mux == nil {
		mux = http.NewServeMux()
	}
	mux.HandleFunc("/ws", s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln

	s.logger.Info("WebSocket server listening", zap.String("addr", addr))

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	return nil
}

func (s *Server) Stop() {
	s.logger.Info("Stopping network server")
	s.cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.httpServer != nil {
		s.logger.Info("Stopping httpServer")
		s.httpServer.Shutdown(ctx)
	}

	s.logger.Info("Close clients")
	s.clientsMu.Lock()
	clients := make([]*Client, 0, len(s.clients))
	for _, c := range s.clients {
		clients = append(clients, c)
	}
	s.clientsMu.Unlock()

	for _, c := range clients {
		c.Close()
	}

	s.wg.Wait()
	s.logger.Info("WebSocket server stopped")
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}

	clientID := s.nextID.Add(1)
	client := &Client{
		ID:      clientID,
		conn:    conn,
		server:  s,
		logger:  s.logger.Named("client").With(zap.Uint64("id", clientID)),
		sendCh:  make(chan []byte, 32768),
		closeCh: make(chan struct{}),
	}

	s.clientsMu.Lock()
	s.clients[client.ID] = client
	s.clientsMu.Unlock()

	if s.onConnect != nil {
		s.onConnect(client)
	}

	s.wg.Add(2)
	go client.readLoop()
	go client.writeLoop()
}

func (c *Client) readLoop() {
	defer c.server.wg.Done()
	defer c.Close()

	for {
		err := c.conn.SetReadDeadline(time.Now().Add(c.server.cfg.ReadTimeout))
		if err != nil {
			c.logger.Error("Failed to set read deadline", zap.Uint64("client_id", c.ID), zap.Error(err))
			return
		}

		msg, op, err := wsutil.ReadClientData(c.conn)
		if err != nil {
			return
		}

		if op == ws.OpBinary && c.server.onMessage != nil {
			c.server.onMessage(c, msg)
		}

		select {
		case <-c.closeCh:
			return
		case <-c.server.ctx.Done():
			return
		default:
		}
	}
}

func (c *Client) writeLoop() {
	defer c.server.wg.Done()
	defer c.Close()

	for {
		select {
		case <-c.closeCh:
			return
		case <-c.server.ctx.Done():
			return
		case msg := <-c.sendCh:
			c.conn.SetWriteDeadline(time.Now().Add(c.server.cfg.WriteTimeout))
			if err := wsutil.WriteServerBinary(c.conn, msg); err != nil {
				return
			}
		}
	}
}

func (c *Client) Send(data []byte) {
	select {
	case c.sendCh <- data:
	default:
		c.server.logger.Warn("Client send buffer full, dropping message", zap.Uint64("client_id", c.ID))
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		c.conn.Close()

		c.server.clientsMu.Lock()
		delete(c.server.clients, c.ID)
		c.server.clientsMu.Unlock()

		if c.server.onDisconnect != nil {
			c.server.onDisconnect(c)
		}
	})
}

func (c *Client) Done() <-chan struct{} {
	return c.closeCh
}

func (s *Server) GetClient(id uint64) *Client {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return s.clients[id]
}

func (s *Server) ClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

func (s *Server) Broadcast(data []byte) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	for _, c := range s.clients {
		c.Send(data)
	}
}
