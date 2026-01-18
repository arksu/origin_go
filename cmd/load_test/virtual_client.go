package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	netproto "origin/internal/network/proto"
	"origin/internal/persistence"
)

type VirtualClient struct {
	cfg         *Config
	db          *persistence.Postgres
	accountPool *AccountPool
	metrics     *Metrics
	logger      *zap.Logger

	account     *AccountInfo
	token       string
	authToken   string
	characterID int64

	conn     io.ReadWriteCloser
	sequence uint32

	playerEntityID uint64
	playerX        int32
	playerY        int32

	mu       sync.Mutex
	entities map[uint64]*EntityState

	enterWorldCh chan struct{}
	stopCh       chan struct{}
}

type EntityState struct {
	EntityID uint64
	X        int32
	Y        int32
}

func NewVirtualClient(cfg *Config, db *persistence.Postgres, pool *AccountPool, metrics *Metrics, logger *zap.Logger) *VirtualClient {
	return &VirtualClient{
		cfg:          cfg,
		db:           db,
		accountPool:  pool,
		metrics:      metrics,
		logger:       logger,
		entities:     make(map[uint64]*EntityState),
		enterWorldCh: make(chan struct{}),
		stopCh:       make(chan struct{}),
	}
}

func (vc *VirtualClient) Run(ctx context.Context) error {
	defer vc.cleanup()

	if err := vc.acquireAccount(ctx); err != nil {
		return fmt.Errorf("acquire account: %w", err)
	}

	if err := vc.login(ctx); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	if err := vc.ensureCharacter(ctx); err != nil {
		return fmt.Errorf("ensure character: %w", err)
	}

	if err := vc.enterCharacter(ctx); err != nil {
		return fmt.Errorf("enter character: %w", err)
	}

	if err := vc.connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	if err := vc.authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	if err := vc.waitEnterWorld(ctx); err != nil {
		return fmt.Errorf("wait enter world: %w", err)
	}

	if vc.cfg.Scenario == "login-only" {
		<-ctx.Done()
		return nil
	}

	return vc.moveLoop(ctx)
}

func (vc *VirtualClient) cleanup() {
	close(vc.stopCh)
	if vc.conn != nil {
		vc.conn.Close()
	}
	vc.accountPool.Release(vc.account)
}

func (vc *VirtualClient) acquireAccount(ctx context.Context) error {
	acc, err := vc.accountPool.Acquire(ctx)
	if err != nil {
		return err
	}
	vc.account = acc
	vc.logger.Debug("Acquired account", zap.String("login", acc.Login))
	return nil
}

func (vc *VirtualClient) login(ctx context.Context) error {
	vc.metrics.RecordLoginAttempt()
	start := time.Now()

	url := fmt.Sprintf("http://%s:%d/accounts/login", vc.cfg.ServerHost, vc.cfg.ServerPort)
	body, _ := json.Marshal(map[string]string{
		"login":    vc.account.Login,
		"password": vc.account.Password,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		vc.metrics.RecordLoginFailure()
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		vc.metrics.RecordLoginFailure()
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		vc.metrics.RecordLoginFailure()
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed: %s - %s", resp.Status, string(respBody))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		vc.metrics.RecordLoginFailure()
		return err
	}

	vc.token = result.Token
	vc.metrics.RecordLoginSuccess(time.Since(start))
	vc.logger.Debug("Login successful")
	return nil
}

func (vc *VirtualClient) ensureCharacter(ctx context.Context) error {
	characters, err := vc.listCharacters(ctx)
	if err != nil {
		return err
	}

	if len(characters) > 0 {
		idx := rand.Intn(len(characters))
		vc.characterID = characters[idx].ID
		vc.logger.Debug("Using existing character", zap.Int64("id", vc.characterID))
		return nil
	}

	return vc.createCharacter(ctx)
}

type CharacterItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (vc *VirtualClient) listCharacters(ctx context.Context) ([]CharacterItem, error) {
	url := fmt.Sprintf("http://%s:%d/characters", vc.cfg.ServerHost, vc.cfg.ServerPort)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+vc.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list characters failed: %s", resp.Status)
	}

	var result struct {
		List []CharacterItem `json:"list"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.List, nil
}

func (vc *VirtualClient) createCharacter(ctx context.Context) error {
	name := generateRandomLogin(8)
	url := fmt.Sprintf("http://%s:%d/characters", vc.cfg.ServerHost, vc.cfg.ServerPort)

	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+vc.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("create character failed: %s", resp.Status)
	}

	characters, err := vc.listCharacters(ctx)
	if err != nil {
		return err
	}
	if len(characters) == 0 {
		return errors.New("no characters after creation")
	}

	vc.characterID = characters[0].ID
	vc.logger.Debug("Created character", zap.Int64("id", vc.characterID), zap.String("name", name))
	return nil
}

func (vc *VirtualClient) enterCharacter(ctx context.Context) error {
	url := fmt.Sprintf("http://%s:%d/characters/%d/enter", vc.cfg.ServerHost, vc.cfg.ServerPort, vc.characterID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+vc.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("enter character failed: %s", resp.Status)
	}

	var result struct {
		AuthToken string `json:"auth_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	vc.authToken = result.AuthToken
	vc.logger.Debug("Got auth token for character")
	return nil
}

func (vc *VirtualClient) connect(ctx context.Context) error {
	url := fmt.Sprintf("ws://%s:%d/ws", vc.cfg.ServerHost, vc.cfg.ServerPort)

	conn, _, _, err := ws.Dial(ctx, url)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}

	vc.conn = conn
	vc.logger.Debug("WebSocket connected")

	go vc.readLoop()

	return nil
}

func (vc *VirtualClient) authenticate(ctx context.Context) error {
	vc.metrics.RecordEnterWorldAttempt()

	msg := &netproto.ClientMessage{
		Sequence: vc.nextSequence(),
		Payload: &netproto.ClientMessage_Auth{
			Auth: &netproto.C2S_Auth{
				Token: vc.authToken,
			},
		},
	}

	return vc.sendMessage(msg)
}

func (vc *VirtualClient) waitEnterWorld(ctx context.Context) error {
	start := time.Now()

	select {
	case <-ctx.Done():
		vc.metrics.RecordEnterWorldFailure()
		return ctx.Err()
	case <-vc.enterWorldCh:
		vc.metrics.RecordEnterWorldSuccess(time.Since(start))
		vc.logger.Debug("Entered world", zap.Uint64("entity_id", vc.playerEntityID))
		return nil
	case <-time.After(30 * time.Second):
		vc.metrics.RecordEnterWorldFailure()
		return errors.New("timeout waiting for enter world")
	}
}

func (vc *VirtualClient) moveLoop(ctx context.Context) error {
	ticker := time.NewTicker(vc.cfg.Period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-vc.stopCh:
			return nil
		case <-ticker.C:
			if err := vc.sendMoveCommand(); err != nil {
				vc.logger.Debug("Failed to send move", zap.Error(err))
				vc.metrics.RecordError()
			}
		}
	}
}

func (vc *VirtualClient) sendMoveCommand() error {
	vc.mu.Lock()
	currentX := vc.playerX
	currentY := vc.playerY
	vc.mu.Unlock()

	dx := int32(rand.Intn(vc.cfg.MoveRadius*2+1) - vc.cfg.MoveRadius)
	dy := int32(rand.Intn(vc.cfg.MoveRadius*2+1) - vc.cfg.MoveRadius)

	targetX := currentX + dx
	targetY := currentY + dy

	msg := &netproto.ClientMessage{
		Sequence: vc.nextSequence(),
		Payload: &netproto.ClientMessage_PlayerAction{
			PlayerAction: &netproto.C2S_PlayerAction{
				Action: &netproto.C2S_PlayerAction_MoveTo{
					MoveTo: &netproto.MoveTo{
						X: targetX,
						Y: targetY,
					},
				},
			},
		},
	}

	if err := vc.sendMessage(msg); err != nil {
		return err
	}

	vc.metrics.RecordMoveSent()
	return nil
}

func (vc *VirtualClient) readLoop() {
	for {
		select {
		case <-vc.stopCh:
			return
		default:
		}

		data, err := wsutil.ReadServerBinary(vc.conn)
		if err != nil {
			select {
			case <-vc.stopCh:
				return
			default:
				vc.logger.Debug("Read error", zap.Error(err))
				return
			}
		}

		vc.metrics.RecordPacketReceived()
		vc.handleMessage(data)
	}
}

func (vc *VirtualClient) handleMessage(data []byte) {
	msg := &netproto.ServerMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		vc.logger.Debug("Failed to unmarshal message", zap.Error(err))
		return
	}

	switch payload := msg.Payload.(type) {
	case *netproto.ServerMessage_AuthResult:
		if !payload.AuthResult.Success {
			vc.logger.Warn("Auth failed", zap.String("error", payload.AuthResult.ErrorMessage))
			vc.metrics.RecordEnterWorldFailure()
		}

	case *netproto.ServerMessage_PlayerEnterWorld:
		vc.mu.Lock()
		vc.playerEntityID = payload.PlayerEnterWorld.EntityId
		if payload.PlayerEnterWorld.Movement != nil && payload.PlayerEnterWorld.Movement.Position != nil {
			vc.playerX = payload.PlayerEnterWorld.Movement.Position.X
			vc.playerY = payload.PlayerEnterWorld.Movement.Position.Y
		}
		vc.mu.Unlock()

		select {
		case vc.enterWorldCh <- struct{}{}:
		default:
		}

	case *netproto.ServerMessage_ObjectSpawn:
		vc.mu.Lock()
		vc.entities[payload.ObjectSpawn.EntityId] = &EntityState{
			EntityID: payload.ObjectSpawn.EntityId,
			X:        payload.ObjectSpawn.Position.Position.X,
			Y:        payload.ObjectSpawn.Position.Position.Y,
		}
		if payload.ObjectSpawn.EntityId == vc.playerEntityID {
			vc.playerX = payload.ObjectSpawn.Position.Position.X
			vc.playerY = payload.ObjectSpawn.Position.Position.Y
		}
		vc.mu.Unlock()

	case *netproto.ServerMessage_ObjectMove:
		vc.mu.Lock()
		if ent, ok := vc.entities[payload.ObjectMove.EntityId]; ok {
			if payload.ObjectMove.Movement != nil && payload.ObjectMove.Movement.Position != nil {
				ent.X = payload.ObjectMove.Movement.Position.X
				ent.Y = payload.ObjectMove.Movement.Position.Y
			}
		}
		if payload.ObjectMove.EntityId == vc.playerEntityID {
			vc.metrics.RecordMoveReceived()
			if payload.ObjectMove.Movement != nil && payload.ObjectMove.Movement.Position != nil {
				vc.playerX = payload.ObjectMove.Movement.Position.X
				vc.playerY = payload.ObjectMove.Movement.Position.Y
			}
		}
		vc.mu.Unlock()

	case *netproto.ServerMessage_Error:
		vc.logger.Debug("Server error", zap.String("message", payload.Error.Message))
		vc.metrics.RecordError()
	}
}

func (vc *VirtualClient) sendMessage(msg *netproto.ClientMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if err := wsutil.WriteClientBinary(vc.conn, data); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	vc.metrics.RecordPacketSent()
	return nil
}

func (vc *VirtualClient) nextSequence() uint32 {
	vc.sequence++
	return vc.sequence
}
