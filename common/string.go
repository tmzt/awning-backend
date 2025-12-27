package common

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

func RandomID() string {
	u, _ := uuid.NewRandom()
	return u.String()
}

var safeStringRegex = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func SafeString(s string) string {
	return safeStringRegex.ReplaceAllString(strings.ToLower(s), "_")
}
