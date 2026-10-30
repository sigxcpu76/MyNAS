//go:build !cgo

package identity

import (
	"fmt"
)

// PAMSupported reports whether this build can authenticate logins via PAM.
const PAMSupported = false

// AuthenticatePAM is a stub when CGO is disabled.
func AuthenticatePAM(serviceName, username, password string) error {
	return fmt.Errorf("cgo is disabled: PAM authentication requires cgo")
}
