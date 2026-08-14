package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"
)

const sessionCookieName = "concierge_session"
const sessionTTL = 2 * time.Hour

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func newSessionID() (string, error) { return randomHex(32) }

func createSession(db *sql.DB, w http.ResponseWriter) error {
	id, err := newSessionID()
	if err != nil {
		return err
	}
	now := time.Now()
	expires := now.Add(sessionTTL)
	if _, err := db.Exec(`INSERT INTO sessions (id, created_at, expires_at) VALUES (?, ?, ?)`, id, now, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func destroySession(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		db.Exec(`DELETE FROM sessions WHERE id = ?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func validSession(db *sql.DB, r *http.Request) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	var expires time.Time
	err = db.QueryRow(`SELECT expires_at FROM sessions WHERE id = ?`, c.Value).Scan(&expires)
	if err != nil {
		return false
	}
	return time.Now().Before(expires)
}

// requireAuth wraps an admin handler, redirecting to /login when there is no
// valid session. TOTP secret setup/reset is CLI-only (see totp.go) — this
// middleware never grants a way around that.
func requireAuth(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validSession(db, r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
