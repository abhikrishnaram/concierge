package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pquerna/otp/totp"
)

// runTOTPReset implements `concierge totp-reset`: generates a new TOTP secret,
// persists it to DATA_DIR, and prints the secret + otpauth:// URI for the
// authenticator app. Never reachable over HTTP, by design — see spec.
func runTOTPReset() {
	if err := os.MkdirAll(dataDir(), 0700); err != nil {
		fmt.Fprintln(os.Stderr, "creating data dir:", err)
		os.Exit(1)
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer(),
		AccountName: "admin",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "generating secret:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(totpSecretPath(), []byte(key.Secret()), 0600); err != nil {
		fmt.Fprintln(os.Stderr, "writing secret:", err)
		os.Exit(1)
	}
	fmt.Println("New TOTP secret generated and saved to", totpSecretPath())
	fmt.Println()
	fmt.Println("Secret:  ", key.Secret())
	fmt.Println("otpauth URI:", key.URL())
	fmt.Println()
	fmt.Println("Scan the URI (as a QR code) or enter the secret manually into your authenticator app.")
}

func loadTOTPSecret() (string, error) {
	b, err := os.ReadFile(totpSecretPath())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func verifyTOTP(code string) bool {
	secret, err := loadTOTPSecret()
	if err != nil {
		return false
	}
	return totp.Validate(code, secret)
}
