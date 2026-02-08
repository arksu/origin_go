package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"origin/internal/persistence"
	"origin/internal/persistence/repository"
)

type AccountInfo struct {
	ID          int64
	Login       string
	Password    string
	Token       string
	CharacterID int64 // pre-selected offline character (0 = no character, need to create)
}

type AccountPool struct {
	db     *persistence.Postgres
	logger *zap.Logger

	mu        sync.Mutex
	accounts  []AccountInfo
	accountAt int // next account index to try
}

func NewAccountPool(db *persistence.Postgres, logger *zap.Logger) *AccountPool {
	return &AccountPool{
		db:     db,
		logger: logger,
	}
}

func (p *AccountPool) Init(ctx context.Context) error {
	// Single query: find lt_ accounts that have at least one offline character,
	// or have no characters at all (a new character will be created later).
	// Accounts where ALL characters are online are excluded.
	const query = `
		SELECT a.id, a.login, COALESCE(c.id, 0) AS character_id
		FROM account a
		LEFT JOIN LATERAL (
			SELECT c.id
			FROM character c
			WHERE c.account_id = a.id
			  AND c.deleted_at IS NULL
			  AND (c.is_online IS NULL OR c.is_online = false)
			LIMIT 1
		) c ON true
		WHERE a.login LIKE 'lt_%'
		  AND (
		      c.id IS NOT NULL
		      OR NOT EXISTS (
		          SELECT 1 FROM character
		          WHERE account_id = a.id AND deleted_at IS NULL
		      )
		  )
		ORDER BY c.id DESC NULLS LAST
	`

	rows, err := p.db.Pool().Query(ctx, query)
	if err != nil {
		return fmt.Errorf("load available accounts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var info AccountInfo
		if err := rows.Scan(&info.ID, &info.Login, &info.CharacterID); err != nil {
			return fmt.Errorf("scan account row: %w", err)
		}
		info.Password = "123"
		p.accounts = append(p.accounts, info)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate account rows: %w", err)
	}

	p.logger.Info("Loaded available load test accounts", zap.Int("count", len(p.accounts)))
	return nil
}

func (p *AccountPool) Acquire(ctx context.Context) (*AccountInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.accountAt < len(p.accounts) {
		acc := &p.accounts[p.accountAt]
		p.accountAt++
		return acc, nil
	}

	// All pre-loaded accounts exhausted — create a new one
	acc, err := p.createAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	p.accounts = append(p.accounts, *acc)
	p.accountAt++
	return acc, nil
}

func (p *AccountPool) Release(acc *AccountInfo) {
	// No-op: the game server manages is_online state on disconnect
}

func (p *AccountPool) createAccount(ctx context.Context) (*AccountInfo, error) {
	login := generateRandomLogin(6)
	password := "123"

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	account, err := p.db.Queries().CreateAccount(ctx, repository.CreateAccountParams{
		Login:        login,
		PasswordHash: string(hash),
	})
	if err != nil {
		return nil, fmt.Errorf("create account in db: %w", err)
	}

	token, err := generateToken(64)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	if err := p.db.Queries().UpdateAccountToken(ctx, repository.UpdateAccountTokenParams{
		Token: sql.NullString{String: token, Valid: true},
		ID:    account.ID,
	}); err != nil {
		return nil, fmt.Errorf("update account token: %w", err)
	}

	p.logger.Debug("Created test account", zap.String("login", login), zap.Int64("id", account.ID))

	return &AccountInfo{
		ID:       account.ID,
		Login:    login,
		Password: password,
		Token:    token,
	}, nil
}

func generateRandomLogin(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	rand.Read(b)
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return "lt_" + string(b)
}

func generateToken(length int) (string, error) {
	b := make([]byte, length/2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
