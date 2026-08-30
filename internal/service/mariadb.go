package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const databasePasswordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// GenerateDatabasePassword creates a URL- and shell-friendly secret that is
// never stored by provctl. Callers show it once or write it to an explicit file.
func GenerateDatabasePassword(length int) (string, error) {
	if length < 24 {
		return "", fmt.Errorf("database password length must be at least 24")
	}
	password := make([]byte, length)
	limit := big.NewInt(int64(len(databasePasswordAlphabet)))
	for index := range password {
		value, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate database password: %w", err)
		}
		password[index] = databasePasswordAlphabet[value.Int64()]
	}
	return string(password), nil
}
