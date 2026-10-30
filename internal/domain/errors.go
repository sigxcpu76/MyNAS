package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrNotFound           = errors.New("resource not found")
	ErrAlreadyExists      = errors.New("resource already exists")
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrFailedPrecondition = errors.New("failed precondition")
)

var resourceIDRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// ValidateResourceID ensures a resource identifier adheres to AIP-122/Linux naming conventions.
func ValidateResourceID(id, fieldName string) error {
	if !resourceIDRegex.MatchString(id) {
		return fmt.Errorf("%w: %s %q must start with an alphanumeric character and contain only [a-zA-Z0-9._-]", ErrInvalidArgument, fieldName, id)
	}
	return nil
}

// ValidateResourcePath validates a '/'-separated identifier (e.g. a nested ZFS
// dataset "media/movies" or a full resource name "zfsPools/tank/datasets/media"),
// requiring every segment to be a valid resource ID.
func ValidateResourcePath(path, fieldName string) error {
	if len(path) > 1024 {
		return fmt.Errorf("%w: %s is too long", ErrInvalidArgument, fieldName)
	}
	for _, segment := range strings.Split(path, "/") {
		if err := ValidateResourceID(segment, fieldName); err != nil {
			return fmt.Errorf("%w: %s %q contains an invalid segment %q", ErrInvalidArgument, fieldName, path, segment)
		}
	}
	return nil
}
