package main

import "os"

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func port() string       { return envOr("PORT", "8080") }
func dataDir() string    { return envOr("DATA_DIR", "/data") }
func totpIssuer() string { return envOr("TOTP_ISSUER", "Concierge") }
func hostExec() bool     { return envOr("HOST_EXEC", "") != "" }

func dbPath() string         { return dataDir() + "/concierge.db" }
func totpSecretPath() string { return dataDir() + "/totp_secret" }
