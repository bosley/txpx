package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	CSRFTokenExpiration = 30 * time.Minute
	SessionCookieName   = "txpx_session"
)

var (
	ErrInvalidCSRF   = errors.New("invalid CSRF token")
	ErrCSRFExpired   = errors.New("CSRF token expired")
	ErrCSRFNotIssued = errors.New("CSRF token not issued")
)

type UserSession struct {
	UUID           string         `json:"uuid"`
	UserUUID       string         `json:"user_uuid"`
	NextCSRF       string         `json:"next_csrf"` // The next expected CSRF token
	LastIssuedCSRF time.Time      `json:"last_issued_csrf"`
	ExpiresAt      time.Time      `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
	Decorations    map[string]any `json:"decorations"`
}

func (x *UserSession) ValidateCSRF(csrf string) error {
	if x.LastIssuedCSRF.IsZero() {
		return ErrCSRFNotIssued
	}
	if time.Now().After(x.LastIssuedCSRF.Add(CSRFTokenExpiration)) {
		return ErrCSRFExpired
	}
	if x.NextCSRF != csrf {
		return ErrInvalidCSRF
	}
	return nil
}

func (x *UserSession) BumpCSRF() {
	val := fmt.Sprintf("%s:%s:%s", x.UUID, x.UserUUID, time.Now().Format(time.RFC3339))
	hash := sha256.Sum256([]byte(val))
	x.NextCSRF = hex.EncodeToString(hash[:])
	x.LastIssuedCSRF = time.Now()
}

func (x *UserSession) SetCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    x.UUID,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		Expires:  x.ExpiresAt,
	})
}

func (x *UserSession) DeleteCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		MaxAge:   -1,
	})
}
