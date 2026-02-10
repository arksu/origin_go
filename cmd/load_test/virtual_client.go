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
	"sync/atomic"
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

	playerEntityID atomic.Uint64
	playerX        atomic.Int32
	playerY        atomic.Int32

	// Reused protobuf objects for outgoing and fallback incoming decode.
	authMsg    netproto.ClientMessage
	moveMsg    netproto.ClientMessage
	incoming   netproto.ServerMessage
	marshalBuf []byte

	decodedPackets uint32

	enterWorldCh chan struct{}
	stopCh       chan struct{}
}

func NewVirtualClient(cfg *Config, db *persistence.Postgres, pool *AccountPool, metrics *Metrics, logger *zap.Logger) *VirtualClient {
	vc := &VirtualClient{
		cfg:          cfg,
		db:           db,
		accountPool:  pool,
		metrics:      metrics,
		logger:       logger,
		enterWorldCh: make(chan struct{}),
		stopCh:       make(chan struct{}),
	}
	vc.authMsg = netproto.ClientMessage{
		Payload: &netproto.ClientMessage_Auth{
			Auth: &netproto.C2S_Auth{},
		},
	}
	vc.moveMsg = netproto.ClientMessage{
		Payload: &netproto.ClientMessage_PlayerAction{
			PlayerAction: &netproto.C2S_PlayerAction{
				Action: &netproto.C2S_PlayerAction_MoveTo{
					MoveTo: &netproto.MoveTo{},
				},
			},
		},
	}
	return vc
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

	url := fmt.Sprintf("%s://%s:%d%s/accounts/login", vc.httpScheme(), vc.cfg.ServerHost, vc.cfg.ServerPort, vc.apiPrefix())
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
	if vc.account.CharacterID > 0 {
		vc.characterID = vc.account.CharacterID
		vc.logger.Debug("Using pre-selected offline character", zap.Int64("id", vc.characterID))
		return nil
	}

	return vc.createCharacter(ctx)
}

type CharacterItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (vc *VirtualClient) listCharacters(ctx context.Context) ([]CharacterItem, error) {
	url := fmt.Sprintf("%s://%s:%d%s/characters", vc.httpScheme(), vc.cfg.ServerHost, vc.cfg.ServerPort, vc.apiPrefix())

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
	url := fmt.Sprintf("%s://%s:%d%s/characters", vc.httpScheme(), vc.cfg.ServerHost, vc.cfg.ServerPort, vc.apiPrefix())

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
	url := fmt.Sprintf("%s://%s:%d%s/characters/%d/enter", vc.httpScheme(), vc.cfg.ServerHost, vc.cfg.ServerPort, vc.apiPrefix(), vc.characterID)

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
	scheme := "ws"
	if vc.cfg.ServerPort == 443 {
		scheme = "wss"
	}
	url := fmt.Sprintf("%s://%s:%d/ws", scheme, vc.cfg.ServerHost, vc.cfg.ServerPort)

	conn, _, _, err := ws.Dial(ctx, url)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}

	vc.conn = conn
	vc.logger.Debug("WebSocket connected")

	go vc.readLoop()

	return nil
}

func (vc *VirtualClient) httpScheme() string {
	if vc.cfg.ServerPort == 443 {
		return "https"
	}
	return "http"
}

func (vc *VirtualClient) apiPrefix() string {
	if vc.cfg.ServerPort == 443 {
		return "/api"
	}
	return ""
}

func (vc *VirtualClient) authenticate(ctx context.Context) error {
	vc.metrics.RecordEnterWorldAttempt()

	vc.authMsg.Sequence = vc.nextSequence()
	if payload, ok := vc.authMsg.Payload.(*netproto.ClientMessage_Auth); ok && payload.Auth != nil {
		payload.Auth.Token = vc.authToken
	}

	return vc.sendMessage(&vc.authMsg)
}

func (vc *VirtualClient) waitEnterWorld(ctx context.Context) error {
	start := time.Now()

	select {
	case <-ctx.Done():
		vc.metrics.RecordEnterWorldFailure()
		return ctx.Err()
	case <-vc.enterWorldCh:
		vc.metrics.RecordEnterWorldSuccess(time.Since(start))
		vc.logger.Debug("Entered world", zap.Uint64("entity_id", vc.playerEntityID.Load()))
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
	currentX := vc.playerX.Load()
	currentY := vc.playerY.Load()

	dx := int32(rand.Intn(vc.cfg.MoveRadius*2+1) - vc.cfg.MoveRadius)
	dy := int32(rand.Intn(vc.cfg.MoveRadius*2+1) - vc.cfg.MoveRadius)

	targetX := currentX + dx
	targetY := currentY + dy

	vc.moveMsg.Sequence = vc.nextSequence()
	if payload, ok := vc.moveMsg.Payload.(*netproto.ClientMessage_PlayerAction); ok && payload.PlayerAction != nil {
		if act, ok := payload.PlayerAction.Action.(*netproto.C2S_PlayerAction_MoveTo); ok && act.MoveTo != nil {
			act.MoveTo.X = targetX
			act.MoveTo.Y = targetY
		}
	}

	if err := vc.sendMessage(&vc.moveMsg); err != nil {
		return err
	}

	vc.metrics.RecordMoveSent()
	return nil
}

func (vc *VirtualClient) readLoop() {
	const sampleMask uint64 = 63 // 1 out of 64 packets
	const decodeLimit uint32 = 100
	const recvMetricsFlushEvery int64 = 1024
	var readSeq uint64
	var localRecvPackets int64
	var localRecvBytes int64
	flushRecvMetrics := func() {
		if localRecvPackets == 0 && localRecvBytes == 0 {
			return
		}
		vc.metrics.RecordPacketReceivedN(localRecvPackets)
		vc.metrics.RecordBytesReceivedN(localRecvBytes)
		localRecvPackets = 0
		localRecvBytes = 0
	}
	defer flushRecvMetrics()
	controlHandler := wsutil.ControlFrameHandler(vc.conn, ws.StateClientSide)
	rd := wsutil.Reader{
		Source:         vc.conn,
		State:          ws.StateClientSide,
		CheckUTF8:      true,
		OnIntermediate: controlHandler,
	}

	for {
		select {
		case <-vc.stopCh:
			return
		default:
		}

		sampled := (readSeq & sampleMask) == 0
		readSeq++

		var readStart time.Time
		if sampled {
			readStart = time.Now()
		}

		hdr, err := rd.NextFrame()
		if err != nil {
			select {
			case <-vc.stopCh:
				return
			default:
				vc.logger.Debug("Read error", zap.Error(err))
				return
			}
		}

		if hdr.OpCode.IsControl() {
			if err := controlHandler(hdr, &rd); err != nil {
				vc.logger.Debug("Control frame handle error", zap.Error(err))
				return
			}
			continue
		}

		if hdr.OpCode != ws.OpBinary {
			if err := rd.Discard(); err != nil {
				vc.logger.Debug("Discard non-binary frame error", zap.Error(err))
				return
			}
			continue
		}

		if vc.decodedPackets >= decodeLimit {
			if err := rd.Discard(); err != nil {
				vc.logger.Debug("Discard frame error", zap.Error(err))
				return
			}
			localRecvPackets++
			if hdr.Length > 0 {
				localRecvBytes += hdr.Length
			}
			if localRecvPackets >= recvMetricsFlushEvery {
				flushRecvMetrics()
			}
			if sampled {
				vc.metrics.RecordReadWait(time.Since(readStart))
			}
			continue
		}

		data, err := io.ReadAll(&rd)
		if err != nil {
			vc.logger.Debug("Read frame payload error", zap.Error(err))
			return
		}

		localRecvPackets++
		localRecvBytes += int64(len(data))
		if localRecvPackets >= recvMetricsFlushEvery {
			flushRecvMetrics()
		}
		if sampled {
			vc.metrics.RecordReadWait(time.Since(readStart))
		}
		if vc.decodedPackets >= decodeLimit {
			continue
		}
		vc.decodedPackets++
		vc.handleMessage(data, sampled)
	}
}

func (vc *VirtualClient) handleMessage(data []byte, sampled bool) {
	var handleStart time.Time
	if sampled {
		handleStart = time.Now()
	}

	var unmarshalStart time.Time
	if sampled {
		unmarshalStart = time.Now()
	}
	vc.incoming.Reset()
	if err := proto.Unmarshal(data, &vc.incoming); err != nil {
		if sampled {
			vc.metrics.RecordUnmarshal(time.Since(unmarshalStart))
		}
		vc.metrics.RecordMsgUnmarshalErr()
		vc.logger.Debug("Failed to unmarshal message", zap.Error(err))
		return
	}
	if sampled {
		vc.metrics.RecordUnmarshal(time.Since(unmarshalStart))
	}

	switch payload := vc.incoming.Payload.(type) {
	case *netproto.ServerMessage_AuthResult:
		vc.metrics.RecordMsgAuthResult()
		if !payload.AuthResult.Success {
			vc.logger.Warn("Auth failed", zap.String("error", payload.AuthResult.ErrorMessage))
			vc.metrics.RecordEnterWorldFailure()
		}

	case *netproto.ServerMessage_PlayerEnterWorld:
		vc.metrics.RecordMsgEnterWorld()
		vc.playerEntityID.Store(payload.PlayerEnterWorld.EntityId)

		select {
		case vc.enterWorldCh <- struct{}{}:
		default:
		}

	case *netproto.ServerMessage_ObjectSpawn:
		vc.metrics.RecordMsgObjectSpawn()
		if payload.ObjectSpawn.EntityId == vc.playerEntityID.Load() {
			vc.playerX.Store(payload.ObjectSpawn.Position.Position.X)
			vc.playerY.Store(payload.ObjectSpawn.Position.Position.Y)
		}

	case *netproto.ServerMessage_ObjectMove:
		vc.metrics.RecordMsgObjectMove()
		if payload.ObjectMove.EntityId == vc.playerEntityID.Load() {
			vc.metrics.RecordMoveReceived()
			if payload.ObjectMove.Movement != nil && payload.ObjectMove.Movement.Position != nil {
				vc.playerX.Store(payload.ObjectMove.Movement.Position.X)
				vc.playerY.Store(payload.ObjectMove.Movement.Position.Y)
			}
		}

	case *netproto.ServerMessage_Error:
		vc.metrics.RecordMsgServerError()
		vc.logger.Debug("Server error", zap.String("message", payload.Error.Message))
		vc.metrics.RecordError()
	default:
		vc.metrics.RecordMsgOther()
	}

	if sampled {
		vc.metrics.RecordHandle(time.Since(handleStart))
	}
}

func (vc *VirtualClient) sendMessage(msg *netproto.ClientMessage) error {
	marshalStart := time.Now()
	data, err := proto.MarshalOptions{}.MarshalAppend(vc.marshalBuf[:0], msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	vc.marshalBuf = data
	vc.metrics.RecordSendMarshal(time.Since(marshalStart))

	writeStart := time.Now()
	if err := wsutil.WriteClientBinary(vc.conn, vc.marshalBuf); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	vc.metrics.RecordSendWrite(time.Since(writeStart))

	vc.metrics.RecordPacketSent()
	vc.metrics.RecordBytesSent(len(vc.marshalBuf))
	return nil
}

func (vc *VirtualClient) nextSequence() uint32 {
	vc.sequence++
	return vc.sequence
}
