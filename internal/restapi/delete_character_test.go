package restapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDeleteCharacterRequiresCallback(t *testing.T) {
	require.Panics(t, func() {
		NewHandler(nil, nil, zap.NewNop(), nil, nil)
	})
}

func TestDeleteCharacterRejectsUnauthorizedHeaders(t *testing.T) {
	for _, header := range []string{"", "Basic token", "Bearer", "bearer token"} {
		t.Run(header, func(t *testing.T) {
			called := false
			h := NewHandler(nil, nil, zap.NewNop(), nil, func(context.Context, repository.DeleteCharacterParams) error {
				called = true
				return nil
			})
			mux := http.NewServeMux()
			h.RegisterRoutes(mux)
			r := httptest.NewRequest(http.MethodDelete, "/characters/7", nil)
			r.Header.Set("Authorization", header)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.False(t, called)
		})
	}
}

func TestDeleteCharacterParsesSignedIDsAndPassesAccount(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		pathID string
		id     int64
		status int
	}{
		{name: "invalid", pathID: "not-an-id", status: http.StatusBadRequest},
		{name: "overflow", pathID: "9223372036854775808", status: http.StatusBadRequest},
		{name: "zero", pathID: "0", id: 0, status: http.StatusNoContent},
		{name: "negative", pathID: "-7", id: -7, status: http.StatusNoContent},
		{name: "minimum", pathID: "-9223372036854775808", id: -9223372036854775808, status: http.StatusNoContent},
		{name: "positive", pathID: "7", id: 7, status: http.StatusNoContent},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			called := false
			requestContext := context.WithValue(context.Background(), accountIDKey, int64(42))
			h := NewHandler(nil, nil, zap.NewNop(), nil, func(ctx context.Context, params repository.DeleteCharacterParams) error {
				called = true
				require.Same(t, requestContext, ctx)
				require.Equal(t, repository.DeleteCharacterParams{ID: scenario.id, AccountID: 42}, params)
				return nil
			})
			r := httptest.NewRequest(http.MethodDelete, "/characters/"+scenario.pathID, nil).WithContext(requestContext)
			r.SetPathValue("id", scenario.pathID)
			w := httptest.NewRecorder()
			h.handleDeleteCharacter(w, r)
			require.Equal(t, scenario.status, w.Code)
			require.Equal(t, scenario.status == http.StatusNoContent, called)
			if called {
				require.Empty(t, w.Body.String())
			} else {
				require.JSONEq(t, `{"error":"error","message":"invalid character id"}`, w.Body.String())
			}
		})
	}
}

func TestDeleteCharacterCallbackFailureKeepsInternalErrorResponse(t *testing.T) {
	for _, callbackErr := range []error{errors.New("delete failed"), context.Canceled, context.DeadlineExceeded} {
		t.Run(callbackErr.Error(), func(t *testing.T) {
			h := NewHandler(nil, nil, zap.NewNop(), nil, func(context.Context, repository.DeleteCharacterParams) error {
				return callbackErr
			})
			r := httptest.NewRequest(http.MethodDelete, "/characters/7", nil)
			r = r.WithContext(context.WithValue(r.Context(), accountIDKey, int64(42)))
			r.SetPathValue("id", "7")
			w := httptest.NewRecorder()
			h.handleDeleteCharacter(w, r)
			require.Equal(t, http.StatusInternalServerError, w.Code)
			require.JSONEq(t, `{"error":"error","message":"internal error"}`, w.Body.String())
		})
	}
}

func TestDeleteCharacterWaitsForCallbackAfterRequestCancellation(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	h := NewHandler(nil, nil, zap.NewNop(), nil, func(ctx context.Context, _ repository.DeleteCharacterParams) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return nil
	})
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), accountIDKey, int64(42)))
	defer cancel()
	r := httptest.NewRequest(http.MethodDelete, "/characters/7", nil).WithContext(ctx)
	r.SetPathValue("id", "7")
	w := httptest.NewRecorder()
	go func() {
		defer close(done)
		h.handleDeleteCharacter(w, r)
	}()
	t.Cleanup(func() {
		cancel()
		unblock()
		<-done
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("deletion callback did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not reach deletion callback")
	}
	select {
	case <-done:
		t.Fatal("response returned before synchronous deletion completed")
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("response did not return after deletion completed")
	}
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Empty(t, w.Body.String())
}

func TestDeleteCharacterPostgresUsesAuthenticatedAccount(t *testing.T) {
	db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	var accountID int64
	require.NoError(t, db.Pool().QueryRow(context.Background(), `
		INSERT INTO account (login, password_hash, token) VALUES ('delete-test', 'test', 'delete-token') RETURNING id
	`).Scan(&accountID))
	var calls []repository.DeleteCharacterParams
	h := NewHandler(db, nil, zap.NewNop(), nil, func(_ context.Context, params repository.DeleteCharacterParams) error {
		calls = append(calls, params)
		return nil
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	for _, scenario := range []struct {
		token  string
		status int
	}{
		{token: "unknown-token", status: http.StatusUnauthorized},
		{token: "delete-token", status: http.StatusNoContent},
	} {
		r := httptest.NewRequest(http.MethodDelete, "/characters/7", nil)
		r.Header.Set("Authorization", "Bearer "+scenario.token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		require.Equal(t, scenario.status, w.Code)
	}
	require.Equal(t, []repository.DeleteCharacterParams{{ID: 7, AccountID: accountID}}, calls)
}
