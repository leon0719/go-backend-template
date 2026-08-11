// Package jwtutil provides low-level access-token signing/parsing shared by
// internal/accounts and internal/httpserver/middleware. It intentionally has
// no dependency on either of those packages so that accounts (which needs
// middleware for route wiring) and middleware (which needs to validate
// tokens issued by accounts) do not form an import cycle.
package jwtutil

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func NewAccessToken(secret string, userID uuid.UUID, ttl time.Duration) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseAccessToken(secret, tokenStr string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}
	// WithValidMethods pins the accepted signing algorithm to the one
	// NewAccessToken uses. Without it, the keyfunc hands the HMAC secret to
	// whatever `alg` the token claims — the classic algorithm-confusion hole
	// (e.g. a token signed HS256 using an RS256 *public* key as the secret).
	// jwt/v5 already blocks `alg: none`, but this pin is what keeps the
	// template safe if someone later swaps in an asymmetric algorithm.
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return uuid.Nil, errors.New("invalid token")
	}
	return uuid.Parse(claims.Subject)
}
