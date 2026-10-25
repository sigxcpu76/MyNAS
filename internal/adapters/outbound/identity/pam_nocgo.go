//go:build !cgo

package identity

import (
	"fmt"
)

// AuthenticateShadow is a stub when CGO is disabled.
func AuthenticateShadow(username, password string) (bool, error) {
	return false, fmt.Errorf("cgo is disabled: shadow authentication requires cgo")
}

// AuthenticatePAM is a stub when CGO is disabled.
func AuthenticatePAM(serviceName, username, password string) error {
	return fmt.Errorf("cgo is disabled: PAM authentication requires cgo")
}
