package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	jwtgo "github.com/golang-jwt/jwt/v4"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// tokenRefreshWindow is the time window in minutes that we allow to refresh a token. If the token is going to expire in
// less than this time, we generate a new token, otherwise we return the same token.
const tokenRefreshWindow = 3

type JWTManager interface {
	GenerateToken(ctx context.Context, user *User, expiresAt time.Time) (string, error)
	// RefreshToken generates a new token if the current token is going to expire in less than `tokenRefreshWindow` minutes.
	// Otherwise, it returns the same token.
	RefreshToken(ctx context.Context, token string, expiresAt time.Time) (string, error)
	ValidateToken(ctx context.Context, token string) (bool, error)
	GetUserFromToken(ctx context.Context, token string) (*User, error)
	GetTenantIDFromToken(ctx context.Context, token string) (string, error)
}

type claims struct {
	User     *User  `json:"user"`
	TenantID string `json:"tenant_id"`
	jwtgo.RegisteredClaims
}

// defaultJWTManager
type defaultJWTManager struct {
	privateKey string
	publicKey  string
}

func (m *defaultJWTManager) parseToken(tokenString string) (*jwtgo.Token, *claims, error) {
	c := &claims{}
	token, err := jwtgo.ParseWithClaims(tokenString, c, func(t *jwtgo.Token) (interface{}, error) {
		esPublicKey, err := jwtgo.ParseECPublicKeyFromPEM([]byte(m.publicKey))
		if err != nil {
			return nil, fmt.Errorf("parsing EC Public Key: %w", err)
		}

		return esPublicKey, nil
	})
	if err != nil {
		vErr, ok := err.(*jwtgo.ValidationError)
		if !ok {
			return nil, nil, fmt.Errorf("parsing token: %w", err)
		}

		if vErr.Errors == jwtgo.ValidationErrorUnverifiable {
			return nil, nil, fmt.Errorf("invalid key: %w", err)
		}

		return nil, nil, ErrInvalidToken
	}

	return token, c, nil
}

func (m *defaultJWTManager) GenerateToken(ctx context.Context, user *User, expiresAt time.Time) (string, error) {
	esPrivateKey, err := jwtgo.ParseECPrivateKeyFromPEM([]byte(m.privateKey))
	if err != nil {
		return "", fmt.Errorf("parsing EC Private Key: %w", err)
	}

	c := &claims{
		User: user,
		RegisteredClaims: jwtgo.RegisteredClaims{
			ExpiresAt: jwtgo.NewNumericDate(expiresAt),
		},
	}

	// TODO: Always throw this error after migrations are merged [SDP-953]
	currentTenant, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		log.Ctx(ctx).Error(err)
	} else {
		c.TenantID = currentTenant.ID
	}

	token := jwtgo.NewWithClaims(jwtgo.SigningMethodES256, c)

	tokenString, err := token.SignedString(esPrivateKey)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}

	return tokenString, nil
}

// RefreshToken generates a new token if the current token is going to expire in less than `tokenRefreshWindow` minutes.
// Otherwise, it returns the same token.
func (m *defaultJWTManager) RefreshToken(ctx context.Context, tokenString string, expiresAt time.Time) (string, error) {
	_, c, err := m.parseToken(tokenString)
	if err != nil {
		return "", fmt.Errorf("parsing token to be refreshed: %w", err)
	}

	// We only generate new tokens when enough time
	// is elapsed.
	if time.Until(c.ExpiresAt.Time) > tokenRefreshWindow*time.Minute {
		return tokenString, nil
	}

	tokenString, err = m.GenerateToken(ctx, c.User, expiresAt)
	if err != nil {
		return "", fmt.Errorf("generating new refreshed token: %w", err)
	}

	return tokenString, nil
}

func (m *defaultJWTManager) ValidateToken(ctx context.Context, tokenString string) (bool, error) {
	token, _, err := m.parseToken(tokenString)
	if errors.Is(err, ErrInvalidToken) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("parsing token to be validated: %w", err)
	}

	return token.Valid, nil
}

func (m *defaultJWTManager) GetUserFromToken(ctx context.Context, tokenString string) (*User, error) {
	_, c, err := m.parseToken(tokenString)
	if err != nil {
		return nil, fmt.Errorf("parsing token to be validated: %w", err)
	}

	return c.User, nil
}

func (m *defaultJWTManager) GetTenantIDFromToken(ctx context.Context, tokenString string) (string, error) {
	_, c, err := m.parseToken(tokenString)
	if err != nil {
		return "", fmt.Errorf("parsing token to be validated: %w", err)
	}

	return c.TenantID, nil
}

type defaultJWTManagerOption func(m *defaultJWTManager)

func newDefaultJWTManager(options ...defaultJWTManagerOption) *defaultJWTManager {
	jwtManager := &defaultJWTManager{}

	for _, option := range options {
		option(jwtManager)
	}

	return jwtManager
}

func withECKeypair(publicKey string, privateKey string) defaultJWTManagerOption {
	return func(m *defaultJWTManager) {
		m.publicKey = publicKey
		m.privateKey = privateKey
	}
}

// Ensuring that defaultJWTManager is implementing JWTManager interface
var _ JWTManager = (*defaultJWTManager)(nil)
