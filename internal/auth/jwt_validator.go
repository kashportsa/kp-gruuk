package auth

import (
	"context"
	"fmt"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// JWTValidator validates Okta-issued JWT tokens.
type JWTValidator struct {
	issuer   string
	clientID string
	jwks     keyfunc.Keyfunc
	cancel   context.CancelFunc
}

// NewJWTValidator creates a validator that fetches JWKS from the Okta issuer.
func NewJWTValidator(issuer, clientID string) (*JWTValidator, error) {
	jwksURL := issuer + "/v1/keys"

	ctx, cancel := context.WithCancel(context.Background())

	jwks, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}

	return &JWTValidator{
		issuer:   issuer,
		clientID: clientID,
		jwks:     jwks,
		cancel:   cancel,
	}, nil
}

// Validate parses and validates a JWT, returning the email claim.
func (v *JWTValidator) Validate(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, v.jwks.KeyfuncCtx(context.Background()),
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}

	if !token.Valid {
		return "", fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims type")
	}

	// Check audience (cid claim is used by Okta for the client ID)
	if cid, ok := claims["cid"].(string); ok {
		if cid != v.clientID {
			return "", fmt.Errorf("invalid client id in token")
		}
	}

	// Extract email
	email, ok := claims["email"].(string)
	if !ok || email == "" {
		// Try sub as fallback
		email, ok = claims["sub"].(string)
		if !ok || email == "" {
			return "", fmt.Errorf("no email claim in token")
		}
	}

	return email, nil
}

// Close releases resources used by the JWKS fetcher.
func (v *JWTValidator) Close() {
	v.cancel()
}
