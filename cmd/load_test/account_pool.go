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
	ID       int64
	Login    string
	Password string
	Token    string
}

type AccountPool struct {
	db     *persistence.Postgres
	logger *zap.Logger

	mu       sync.Mutex
	inUse    map[int64]bool
	accounts []AccountInfo
}

func NewAccountPool(db *persistence.Postgres, logger *zap.Logger) *AccountPool {
	return &AccountPool{
		db:     db,
		logger: logger,
		inUse:  make(map[int64]bool),
	}
}

func (p *AccountPool) Init(ctx context.Context) error {
	accounts, err := p.db.Queries().GetAllAccounts(ctx)
	if err != nil {
		return fmt.Errorf("get all accounts: %w", err)
	}

	for _, acc := range accounts {
		p.accounts = append(p.accounts, AccountInfo{
			ID:    acc.ID,
			Login: acc.Login,
		})
	}

	p.logger.Info("Loaded accounts from database", zap.Int("count", len(p.accounts)))
	return nil
}

func (p *AccountPool) Acquire(ctx context.Context) (*AccountInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.accounts {
		if !p.inUse[p.accounts[i].ID] {
			p.inUse[p.accounts[i].ID] = true
			return &p.accounts[i], nil
		}
	}

	acc, err := p.createAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	p.accounts = append(p.accounts, *acc)
	p.inUse[acc.ID] = true

	return acc, nil
}

func (p *AccountPool) Release(acc *AccountInfo) {
	if acc == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inUse, acc.ID)
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
