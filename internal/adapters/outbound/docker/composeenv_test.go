package docker

import (
	"strings"
	"testing"
)

func TestEscapeComposeEnvValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"no dollars", "hello_world", "hello_world"},
		{"single dollar", "secret$123", "secret$$123"},
		{"multiple single dollars", "foo$bar$baz", "foo$$bar$$baz"},
		{"already escaped", "secret$$123", "secret$$123"},
		{"bcrypt hash", "$2y$10$abcdef", "$$2y$$10$$abcdef"},
		{"braced interpolation preserved", "${PUID:-1000}", "${PUID:-1000}"},
		{"braced interpolation with dollar after", "${PUID:-1000}_$123", "${PUID:-1000}_$$123"},
		{"trailing dollar", "pass$", "pass$$"},
		{"leading dollar", "$pass", "$$pass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeComposeEnvValue(tt.input)
			if got != tt.expected {
				t.Errorf("EscapeComposeEnvValue(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeComposeEnvironment(t *testing.T) {
	input := `services:
  web:
    image: nginx:alpine
    environment:
      - PASS=secret$123
      - HASH=$2y$10$abcdef
      - SAFE=safe$$pass
      - BRACED=${PUID:-1000}
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: my$db$pass
      SAFE_PASS: already$$escaped
`
	normalized := NormalizeComposeEnvironment(input)

	if !strings.Contains(normalized, "PASS=secret$$123") {
		t.Errorf("expected PASS=secret$$123, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "HASH=$$2y$$10$$abcdef") {
		t.Errorf("expected HASH=$$2y$$10$$abcdef, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "SAFE=safe$$pass") {
		t.Errorf("expected SAFE=safe$$pass, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "BRACED=${PUID:-1000}") {
		t.Errorf("expected BRACED=${PUID:-1000}, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "POSTGRES_PASSWORD: my$$db$$pass") {
		t.Errorf("expected POSTGRES_PASSWORD: my$$db$$pass, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "SAFE_PASS: already$$escaped") {
		t.Errorf("expected SAFE_PASS: already$$escaped, got:\n%s", normalized)
	}
}
