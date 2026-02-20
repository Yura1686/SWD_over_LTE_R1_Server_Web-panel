package auth

import (
	"crypto/subtle"
	"errors"
	"sync"
	"time"

	"lte_swd/backend/server/internal/util"
)

var (
	// ErrInvalidPassword informs caller that operator password mismatched.
	ErrInvalidPassword = errors.New("invalid operator password")
	// ErrInvalidToken informs caller that bearer token is unknown or expired.
	ErrInvalidToken = errors.New("invalid operator token")
)

// OperatorAuth keeps short-lived operator sessions for R1.
type OperatorAuth struct {
	mu       sync.Mutex
	password string
	ttl      time.Duration
	tokens   map[string]time.Time
}

// NewOperatorAuth creates new auth manager.
func NewOperatorAuth(password string, ttl time.Duration) *OperatorAuth {
	return &OperatorAuth{
		password: password,
		ttl:      ttl,
		tokens:   make(map[string]time.Time),
	}
}

// Login validates password and returns bearer token.
func (a *OperatorAuth) Login(password string, now time.Time) (string, time.Time, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) != 1 {
		return "", time.Time{}, ErrInvalidPassword
	}

	token := util.RandomToken("op", 16)
	expiresAt := now.Add(a.ttl)
	a.tokens[token] = expiresAt
	a.cleanupLocked(now)
	return token, expiresAt, nil
}

// Validate checks token validity.
func (a *OperatorAuth) Validate(token string, now time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	expiresAt, ok := a.tokens[token]
	if !ok || now.After(expiresAt) {
		delete(a.tokens, token)
		return ErrInvalidToken
	}
	return nil
}

func (a *OperatorAuth) cleanupLocked(now time.Time) {
	for token, expiresAt := range a.tokens {
		if now.After(expiresAt) {
			delete(a.tokens, token)
		}
	}
}
