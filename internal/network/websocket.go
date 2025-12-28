package network

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/game"
	"origin/internal/network/pb"
	"sync"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"google.golang.org/protobuf/proto"
)

// PacketHandler handles incoming packets by type
// The payload is the raw protobuf bytes (without the 2-byte type header)
type PacketHandler func(c *Client, payload []byte) error

// Client represents a connected WebSocket client
type Client struct {
	conn        net.Conn
	world       *game.World
	server      *Server
	send        chan []byte
	handle      ecs.Handle   // runtime handle for ECS operations
	characterID ecs.EntityID // global unique ID for persistence/replication
	chunkX      int
	chunkY      int
	layer       int
	authed      bool
	mu          sync.RWMutex
}

// Server handles WebSocket connections
type Server struct {
	cfg        *config.Config
	world      *game.World
	clients    map[*Client]bool
	httpServer *http.Server
	handlers   map[PacketType]PacketHandler
	mu         sync.RWMutex
}

// NewServer creates a new WebSocket httpServer
func NewServer(cfg *config.Config, world *game.World) *Server {
	s := &Server{
		cfg:      cfg,
		world:    world,
		clients:  make(map[*Client]bool),
		handlers: make(map[PacketType]PacketHandler),
	}
	s.registerHandlers()
	return s
}

// RegisterHandler registers a packet handler for a specific packet type
func (s *Server) RegisterHandler(packetType PacketType, handler PacketHandler) {
	s.handlers[packetType] = handler
}

// registerHandlers registers all packet handlers
func (s *Server) registerHandlers() {
	s.RegisterHandler(PacketAuth, s.handleAuth)
	s.RegisterHandler(PacketMapClick, s.handleMapClick)
	s.RegisterHandler(PacketObjectClick, s.handleObjectClick)
	s.RegisterHandler(PacketChat, s.handleChat)
}

// Start starts the WebSocket httpServer
func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: mux,
	}

	log.Printf("WebSocket httpServer starting on %s", s.cfg.ListenAddr)
	_ = s.httpServer.ListenAndServe()
}

// Stop gracefully stops the WebSocket httpServer
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	for client := range s.clients {
		client.Close()
	}
	s.mu.Unlock()

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleWebSocket handles WebSocket upgrade and connection
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		conn:   conn,
		world:  s.world,
		server: s,
		send:   make(chan []byte, 256),
	}

	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	log.Printf("Client connected: %s", conn.RemoteAddr())

	go client.writePump()
	go client.readPump()
}

// removeClient removes a client from the httpServer
func (s *Server) removeClient(c *Client) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
}

// Broadcast sends a message to all connected clients
func (s *Server) Broadcast(data []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for client := range s.clients {
		select {
		case client.send <- data:
		default:
			// Client buffer full, skip
		}
	}
}

// readPump reads messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.server.removeClient(c)
		c.Close()
	}()

	for {
		data, op, err := wsutil.ReadClientData(c.conn)
		if err != nil {
			if err != io.EOF {
				log.Printf("Read error: %v", err)
			}
			return
		}

		if op == ws.OpClose {
			return
		}

		if op == ws.OpBinary && len(data) >= 2 {
			c.handlePacket(data)
		}
	}
}

// writePump writes messages to the WebSocket connection
func (c *Client) writePump() {
	defer c.Close()

	for data := range c.send {
		err := wsutil.WriteServerBinary(c.conn, data)
		if err != nil {
			log.Printf("Write error: %v", err)
			return
		}
	}
}

// handlePacket processes an incoming packet
func (c *Client) handlePacket(data []byte) {
	packetType, payload, err := DecodePacket(data)
	if err != nil {
		log.Printf("Decode error: %v", err)
		return
	}

	handler, ok := c.server.handlers[packetType]
	if !ok {
		log.Printf("Unknown packet type: 0x%04X", packetType)
		return
	}

	if err := handler(c, payload); err != nil {
		log.Printf("Packet handler error (type=0x%04X): %v", packetType, err)
	}
}

// Send sends raw data to the client
func (c *Client) Send(data []byte) {
	select {
	case c.send <- data:
	default:
		// Buffer full
	}
}

// SendPacket encodes and sends a protobuf packet to the client
func (c *Client) SendPacket(packetType PacketType, msg proto.Message) error {
	data, err := EncodePacket(packetType, msg)
	if err != nil {
		return err
	}
	c.Send(data)
	return nil
}

// SendError sends an error packet to the client
func (c *Client) SendError(code pb.S2CError_ErrorCode, message string) {
	_ = c.SendPacket(PacketError, &pb.S2CError{
		Code:    code,
		Message: message,
	})
}

// Close closes the client connection
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	select {
	case <-c.send:
	default:
		close(c.send)
	}
}
