package restapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	mathrand "math/rand"
	"net/http"
	"origin/internal/characterattrs"
	"origin/internal/const"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"origin/internal/config"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/game"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
)

type Handler struct {
	db              *persistence.Postgres
	entityIDManager *game.EntityIDManager
	logger          *zap.Logger
	gameConfig      *config.GameConfig
}

func NewHandler(db *persistence.Postgres, entityIDManager *game.EntityIDManager, logger *zap.Logger, gameConfig *config.GameConfig) *Handler {
	return &Handler{
		db:              db,
		entityIDManager: entityIDManager,
		logger:          logger,
		gameConfig:      gameConfig,
	}
}

type contextKey string

const accountIDKey contextKey = "account_id"

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /accounts/registration", h.handleRegistration)
	mux.HandleFunc("POST /accounts/login", h.handleLogin)
	mux.HandleFunc("GET /characters", h.withAuth(h.handleListCharacters))
	mux.HandleFunc("POST /characters", h.withAuth(h.handleCreateCharacter))
	mux.HandleFunc("DELETE /characters/{id}", h.withAuth(h.handleDeleteCharacter))
	mux.HandleFunc("POST /characters/{id}/enter", h.withAuth(h.handleEnterCharacter))
}

func (h *Handler) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			h.jsonError(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			h.jsonError(w, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		token := parts[1]
		account, err := h.db.Queries().GetAccountByToken(r.Context(), sql.NullString{String: token, Valid: true})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				h.jsonError(w, "invalid token", http.StatusUnauthorized)
				return
			}
			h.logger.Error("failed to get account by token", zap.Error(err))
			h.jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), accountIDKey, account.ID)
		next(w, r.WithContext(ctx))
	}
}

func (h *Handler) getAccountID(r *http.Request) int64 {
	return r.Context().Value(accountIDKey).(int64)
}

func (h *Handler) authenticateAndTokenize(r *http.Request, login, password string) (string, error) {
	account, err := h.db.Queries().GetAccountByLogin(r.Context(), login)
	if err != nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)); err != nil {
		return "", err
	}

	token, err := generateToken(64)
	if err != nil {
		return "", err
	}

	if err := h.db.Queries().UpdateAccountToken(r.Context(), repository.UpdateAccountTokenParams{
		Token: sql.NullString{String: token, Valid: true},
		ID:    account.ID,
	}); err != nil {
		return "", err
	}

	return token, nil
}

func (h *Handler) handleRegistration(w http.ResponseWriter, r *http.Request) {
	var req RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		h.jsonError(w, "login and password are required", http.StatusBadRequest)
		return
	}

	_, err := h.db.Queries().GetAccountByLogin(r.Context(), req.Login)
	if err == nil {
		h.jsonError(w, "login already exists", http.StatusConflict)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		h.logger.Error("failed to check login", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("failed to hash password", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	_, err = h.db.Queries().CreateAccount(r.Context(), repository.CreateAccountParams{
		Login:        req.Login,
		PasswordHash: string(hash),
	})
	if err != nil {
		h.logger.Error("failed to create account", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	token, err := h.authenticateAndTokenize(r, req.Login, req.Password)
	if err != nil {
		h.logger.Error("failed to authenticate and tokenize", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, LoginResponse{Token: token}, http.StatusCreated)
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		h.jsonError(w, "login and password are required", http.StatusBadRequest)
		return
	}

	token, err := h.authenticateAndTokenize(r, req.Login, req.Password)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			h.jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		h.logger.Error("failed to authenticate", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, LoginResponse{Token: token}, http.StatusOK)
}

func (h *Handler) handleListCharacters(w http.ResponseWriter, r *http.Request) {
	accountID := h.getAccountID(r)

	characters, err := h.db.Queries().GetCharactersByAccountID(r.Context(), accountID)
	if err != nil {
		h.logger.Error("failed to get characters", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	list := make([]CharacterItem, 0, len(characters))
	for _, c := range characters {
		list = append(list, CharacterItem{ID: c.ID, Name: c.Name})
	}

	h.jsonResponse(w, ListCharactersResponse{List: list}, http.StatusOK)
}

func (h *Handler) handleCreateCharacter(w http.ResponseWriter, r *http.Request) {
	accountID := h.getAccountID(r)

	var req CreateCharacterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		h.jsonError(w, "name is required", http.StatusBadRequest)
		return
	}

	count, err := h.db.Queries().CountCharactersByAccountID(r.Context(), accountID)
	if err != nil {
		h.logger.Error("failed to count characters", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if count >= 5 {
		h.jsonError(w, "maximum 5 characters per account", http.StatusBadRequest)
		return
	}

	id := h.entityIDManager.GetFreeID()

	x, y, err := generateRandomCharacterPosition(h.gameConfig)
	if err != nil {
		h.logger.Error("failed to generate character spawn position", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	defaultAttributes, err := characterattrs.Marshal(characterattrs.Default())
	if err != nil {
		h.logger.Error("failed to marshal default character attributes", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defaultExperience, err := components.MarshalCharacterExperience(components.CharacterExperience{})
	if err != nil {
		h.logger.Error("failed to marshal default character experience", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defaultSkills, err := components.MarshalStringSet(nil)
	if err != nil {
		h.logger.Error("failed to marshal default character skills", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defaultDiscovery, err := components.MarshalStringSet(nil)
	if err != nil {
		h.logger.Error("failed to marshal default character discovery", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	initialStamina := entitystats.MaxStaminaFromCon(characterattrs.DefaultValue)
	initialStaminaDBValue := math.Round(initialStamina)
	initialEnergyDBValue := math.Round(_const.DefaultEnergy)

	_, err = h.db.Queries().CreateCharacter(r.Context(), repository.CreateCharacterParams{
		ID:         int64(id),
		AccountID:  accountID,
		Name:       req.Name,
		X:          x,
		Y:          y,
		Stamina:    initialStaminaDBValue,
		Energy:     initialEnergyDBValue,
		Attributes: defaultAttributes,
		Exp:        defaultExperience,
		Skills:     defaultSkills,
		Discovery:  defaultDiscovery,
	})
	if err != nil {
		h.logger.Error("failed to create character", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func generateRandomCharacterPosition(gameConfig *config.GameConfig) (int, int, error) {
	if gameConfig == nil {
		return 0, 0, errors.New("game config is nil")
	}

	chunkWorldSize := _const.ChunkWorldSize
	marginWorldUnits := gameConfig.WorldMarginTiles * _const.CoordPerTile

	minX := gameConfig.WorldMinXChunks*chunkWorldSize + marginWorldUnits
	maxX := (gameConfig.WorldMinXChunks+gameConfig.WorldWidthChunks)*chunkWorldSize - marginWorldUnits
	minY := gameConfig.WorldMinYChunks*chunkWorldSize + marginWorldUnits
	maxY := (gameConfig.WorldMinYChunks+gameConfig.WorldHeightChunks)*chunkWorldSize - marginWorldUnits

	if maxX <= minX || maxY <= minY {
		return 0, 0, fmt.Errorf(
			"invalid world spawn range: min=(%d,%d) max=(%d,%d), world_min_chunks=(%d,%d), world_size_chunks=(%d,%d), margin_tiles=%d",
			minX, minY, maxX, maxY,
			gameConfig.WorldMinXChunks, gameConfig.WorldMinYChunks,
			gameConfig.WorldWidthChunks, gameConfig.WorldHeightChunks,
			gameConfig.WorldMarginTiles,
		)
	}

	return minX + mathrand.Intn(maxX-minX), minY + mathrand.Intn(maxY-minY), nil
}

func (h *Handler) handleDeleteCharacter(w http.ResponseWriter, r *http.Request) {
	accountID := h.getAccountID(r)

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.jsonError(w, "invalid character id", http.StatusBadRequest)
		return
	}

	if err := h.db.Queries().DeleteCharacter(r.Context(), repository.DeleteCharacterParams{
		ID:        id,
		AccountID: accountID,
	}); err != nil {
		h.logger.Error("failed to delete character", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleEnterCharacter(w http.ResponseWriter, r *http.Request) {
	accountID := h.getAccountID(r)

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.jsonError(w, "invalid character id", http.StatusBadRequest)
		return
	}

	authToken, err := generateToken(64)
	if err != nil {
		h.logger.Error("failed to generate auth token", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(30 * time.Second)
	if err := h.db.Queries().SetCharacterAuthToken(r.Context(), repository.SetCharacterAuthTokenParams{
		ID:             id,
		AuthToken:      sql.NullString{String: authToken, Valid: true},
		TokenExpiresAt: sql.NullTime{Time: expiresAt, Valid: true},
		AccountID:      accountID,
	}); err != nil {
		h.logger.Error("failed to set auth token", zap.Error(err))
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, EnterCharacterResponse{AuthToken: authToken}, http.StatusOK)
}

func (h *Handler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: "error", Message: message})
}

func (h *Handler) jsonResponse(w http.ResponseWriter, data any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func generateToken(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
