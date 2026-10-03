package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/argon2"
	"labreserve.local/labreserve/internal/database"
)

const (
	AnonymousSessionLifetime     = 30 * time.Minute
	AuthenticatedSessionLifetime = 12 * time.Hour
	sessionTokenSize             = 32
)

var ErrInvalidCredentials = errors.New("invalid login or password")
var ErrInvalidSession = errors.New("invalid session")

type Service struct {
	store *database.Store
	now   func() time.Time
}

func NewService(store *database.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, now: now}
}

func (s *Service) StartAnonymous(ctx context.Context) (string, database.Session, error) {
	token, tokenBytes, csrfToken, err := newTokens()
	if err != nil {
		return "", database.Session{}, err
	}
	now := s.now().UTC()
	session := database.Session{
		TokenHash: hashToken(tokenBytes),
		CSRFToken: csrfToken,
		CreatedAt: now,
		ExpiresAt: now.Add(AnonymousSessionLifetime),
	}
	if err := s.store.InsertSession(ctx, session); err != nil {
		return "", database.Session{}, err
	}
	return token, session, nil
}

func (s *Service) Resolve(ctx context.Context, token string) (database.Session, bool, error) {
	tokenBytes, err := decodeToken(token)
	if err != nil {
		return database.Session{}, false, nil
	}
	return s.store.SessionByTokenHash(ctx, hashToken(tokenBytes), s.now().UTC())
}

func (s *Service) Login(ctx context.Context, previousToken, login, password string) (string, database.Session, error) {
	account, found, err := s.store.AccountByLogin(ctx, login)
	if err != nil {
		return "", database.Session{}, err
	}
	if !found {
		dummyPasswordWork(password)
		return "", database.Session{}, ErrInvalidCredentials
	}
	if !VerifyPassword(account.PasswordHash, password) {
		return "", database.Session{}, ErrInvalidCredentials
	}
	previousBytes, err := decodeToken(previousToken)
	if err != nil {
		return "", database.Session{}, ErrInvalidSession
	}
	token, tokenBytes, csrfToken, err := newTokens()
	if err != nil {
		return "", database.Session{}, err
	}
	now := s.now().UTC()
	accountID := account.ID
	session := database.Session{
		TokenHash: hashToken(tokenBytes),
		AccountID: &accountID,
		CSRFToken: csrfToken,
		CreatedAt: now,
		ExpiresAt: now.Add(AuthenticatedSessionLifetime),
		Identity: &database.Identity{
			ID:          account.ID,
			Login:       account.Login,
			DisplayName: account.DisplayName,
			Role:        account.Role,
		},
	}
	if err := s.store.RotateSession(ctx, hashToken(previousBytes), session); err != nil {
		return "", database.Session{}, err
	}
	return token, session, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	tokenBytes, err := decodeToken(token)
	if err != nil {
		return ErrInvalidSession
	}
	return s.store.DeleteSession(ctx, hashToken(tokenBytes))
}

func CSRFToken(session database.Session) string {
	return base64.RawURLEncoding.EncodeToString(session.CSRFToken)
}

func ValidCSRF(session database.Session, submitted string) bool {
	if len(submitted) != base64.RawURLEncoding.EncodedLen(sessionTokenSize) {
		return false
	}
	provided, err := base64.RawURLEncoding.DecodeString(submitted)
	if err != nil || len(provided) != sessionTokenSize || len(session.CSRFToken) != sessionTokenSize {
		return false
	}
	return subtle.ConstantTimeCompare(provided, session.CSRFToken) == 1
}

func newTokens() (string, []byte, []byte, error) {
	tokenBytes := make([]byte, sessionTokenSize)
	csrfToken := make([]byte, sessionTokenSize)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, nil, fmt.Errorf("generate session token: %w", err)
	}
	if _, err := rand.Read(csrfToken); err != nil {
		return "", nil, nil, fmt.Errorf("generate csrf token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(tokenBytes), tokenBytes, csrfToken, nil
}

func decodeToken(token string) ([]byte, error) {
	if len(token) != base64.RawURLEncoding.EncodedLen(sessionTokenSize) {
		return nil, ErrInvalidSession
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != sessionTokenSize {
		return nil, ErrInvalidSession
	}
	return decoded, nil
}

func hashToken(token []byte) []byte {
	digest := sha256.Sum256(token)
	return digest[:]
}

func dummyPasswordWork(password string) {
	key := argon2.IDKey([]byte(password), []byte("labreserve-invalid-account-salt"), passwordIterations, passwordMemory, passwordParallelism, passwordKeyLength)
	subtle.ConstantTimeCompare(key, make([]byte, passwordKeyLength))
}
