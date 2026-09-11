package link

import (
	"crypto/rand"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

const (
	codeLength                = 8
	maxCodeGenerationAttempts = 4
)

var (
	ErrInvalidDestinationURL = errors.New("invalid destination_url")
	ErrInvalidCode           = errors.New("invalid code")
	ErrReservedCode          = errors.New("reserved code")
	ErrCodeConflict          = errors.New("code already exists")
	ErrLinkNotFound          = errors.New("link not found")
	ErrInvalidStatus         = errors.New("invalid status")
)

func validStatus(status string) bool {
	return status == "active" || status == "inactive"
}

// codeAlphabet is URL-safe and excludes ambiguous characters: 0 O, 1 l I.
const codeAlphabet = "23456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"

var codeRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)

var reservedCodes = map[string]struct{}{
	"api":         {},
	"health":      {},
	"favicon.ico": {},
}

func generateCode() (string, error) {
	b := make([]byte, codeLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	max := byte(256 - (256 % len(codeAlphabet)))
	for i := range b {
		for b[i] >= max {
			if _, err := rand.Read(b[i : i+1]); err != nil {
				return "", err
			}
		}
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}

	return string(b), nil
}

func validateDestinationURL(raw string) bool {
	if raw == "" {
		return false
	}

	u, err := url.Parse(raw)
	if err != nil {
		return false
	}

	if u.Host == "" {
		return false
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func validateCustomCode(code string) error {
	if _, ok := reservedCodes[code]; ok {
		return ErrReservedCode
	}
	if !codeRegexp.MatchString(code) {
		return ErrInvalidCode
	}
	return nil
}
