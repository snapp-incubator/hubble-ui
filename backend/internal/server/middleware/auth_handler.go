// Package middleware implements the Dex (OIDC) login flow that makes
// a multi-tenant hubble-ui deployment possible: every request to the API
// must carry a signed JWT in the "token" cookie, which the frontend also
// forwards to hubble-middleware to resolve the namespaces the user is
// authorized to see.
package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/oauth2"
)

const tokenCookieName = "token"

type Config struct {
	// Addr is the URL of the Dex issuer, e.g. https://dex.example.com
	Addr string
	// HubbleURL is the external URL of hubble-ui the user is redirected
	// back to after a successful login
	HubbleURL string
	ClientID  string
	Secret    string
	// JWTExpiration bounds both the login flow state token and the session
	// token lifetime
	JWTExpiration time.Duration
}

type DexAuthHandler struct {
	log *slog.Logger
	cfg Config

	mx       sync.Mutex
	provider *oidc.Provider
}

func NewDex(log *slog.Logger, cfg Config) *DexAuthHandler {
	return &DexAuthHandler{
		log: log,
		cfg: cfg,
	}
}

func (h *DexAuthHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		// NOTE: OAuth callback from Dex: exchange the code for a session
		// token and redirect back to the UI
		if jwtToken, exp := h.dexCallback(req); jwtToken != "" {
			// NOTE: The cookie cannot be HttpOnly: the frontend reads it to
			// authorize its requests to hubble-middleware
			http.SetCookie(resp, &http.Cookie{ //nolint:gosec
				Name:     tokenCookieName,
				Value:    jwtToken,
				Expires:  *exp,
				Path:     "/",
				Secure:   true,
				SameSite: http.SameSiteLaxMode,
			})

			http.Redirect(resp, req, h.cfg.HubbleURL, http.StatusFound)
			return
		}

		if cookie, err := req.Cookie(tokenCookieName); err == nil && h.isValidSessionToken(cookie.Value) {
			next.ServeHTTP(resp, req)
			return
		}

		// NOTE: No valid session: start the login flow with a signed state
		// token so the callback can be validated
		state, err := h.generateStateJWT()
		if err != nil {
			h.log.Error("failed to generate OAuth state token", "error", err)
			http.Error(resp, "server_error", http.StatusInternalServerError)
			return
		}

		oauthCfg, _, err := h.oauthDexConfig(req.Context())
		if err != nil {
			h.log.Error("failed to initialize OIDC provider", "error", err)
			http.Error(resp, "server_error", http.StatusInternalServerError)
			return
		}

		http.Redirect(resp, req, oauthCfg.AuthCodeURL(state), http.StatusFound)
	})
}

func (h *DexAuthHandler) dexCallback(req *http.Request) (string, *time.Time) {
	state := req.URL.Query().Get("state")
	if state == "" {
		return "", nil
	}

	if err := h.validateJWT(state); err != nil {
		h.log.Info("invalid OAuth state token", "error", err)
		return "", nil
	}

	code := req.URL.Query().Get("code")
	if code == "" {
		return "", nil
	}

	oauthCfg, verifier, err := h.oauthDexConfig(req.Context())
	if err != nil {
		h.log.Error("failed to initialize OIDC provider", "error", err)
		return "", nil
	}

	token, err := oauthCfg.Exchange(req.Context(), code)
	if err != nil {
		h.log.Error("OAuth code exchange failed", "error", err)
		return "", nil
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		h.log.Error("OAuth error: no id_token in token response")
		return "", nil
	}

	idToken, err := verifier.Verify(req.Context(), rawIDToken)
	if err != nil {
		h.log.Error("OAuth error: id_token verification failed", "error", err)
		return "", nil
	}

	var claims struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}

	if err := idToken.Claims(&claims); err != nil {
		h.log.Error("OAuth error: failed to parse id_token claims", "error", err)
		return "", nil
	}

	jwtToken, exp, err := h.generateSessionJWT(claims.Email)
	if err != nil {
		h.log.Error("failed to sign session token", "error", err)
		return "", nil
	}

	return jwtToken, exp
}

func (h *DexAuthHandler) oauthDexConfig(ctx context.Context) (
	*oauth2.Config, *oidc.IDTokenVerifier, error,
) {
	provider, err := h.getProvider(ctx)
	if err != nil {
		return nil, nil, err
	}

	return &oauth2.Config{
		RedirectURL:  h.cfg.HubbleURL + "/api/",
		ClientID:     h.cfg.ClientID,
		ClientSecret: h.cfg.Secret,
		Scopes:       []string{"openid", "profile", "email"},
		Endpoint:     provider.Endpoint(),
	}, provider.Verifier(&oidc.Config{ClientID: h.cfg.ClientID}), nil
}

func (h *DexAuthHandler) getProvider(ctx context.Context) (*oidc.Provider, error) {
	h.mx.Lock()
	defer h.mx.Unlock()

	if h.provider != nil {
		return h.provider, nil
	}

	provider, err := oidc.NewProvider(ctx, h.cfg.Addr)
	if err != nil {
		return nil, err
	}

	h.provider = provider
	return provider, nil
}

func (h *DexAuthHandler) isValidSessionToken(tokenString string) bool {
	if tokenString == "" {
		return false
	}

	if err := h.validateJWT(tokenString); err != nil {
		h.log.Debug("session token validation failed", "error", err)
		return false
	}

	return true
}

func (h *DexAuthHandler) validateJWT(tokenString string) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, isHMAC := token.Method.(*jwt.SigningMethodHMAC); !isHMAC {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(h.cfg.Secret), nil
	})

	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("token is not valid")
	}

	return nil
}

func (h *DexAuthHandler) generateStateJWT() (string, error) {
	token := jwt.New(jwt.SigningMethodHS512)
	claims := token.Claims.(jwt.MapClaims)
	claims["exp"] = time.Now().Add(h.cfg.JWTExpiration).Unix()

	return token.SignedString([]byte(h.cfg.Secret))
}

func (h *DexAuthHandler) generateSessionJWT(username string) (string, *time.Time, error) {
	token := jwt.New(jwt.SigningMethodHS512)
	claims := token.Claims.(jwt.MapClaims)

	iat := time.Now()
	exp := iat.Add(h.cfg.JWTExpiration)

	claims["username"] = username
	claims["iat"] = iat.Unix()
	claims["exp"] = exp.Unix()

	tokenString, err := token.SignedString([]byte(h.cfg.Secret))
	if err != nil {
		return "", nil, err
	}

	return tokenString, &exp, nil
}
