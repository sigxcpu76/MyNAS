package grpcadapter

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"github.com/sigxcpu/nas/internal/domain"
	"go.einride.tech/aip/resourcename"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ParseSingleResourceID extracts `{id}` from a single-collection AIP resource name like `zfsPools/{zfs_pool}`.
func ParseSingleResourceID(pattern, name string) (string, error) {
	if err := resourcename.Validate(name); err != nil {
		return "", status.Errorf(codes.InvalidArgument, "invalid resource name %q: %v", name, err)
	}
	if !resourcename.Match(pattern, name) {
		return "", status.Errorf(codes.InvalidArgument, "resource name %q does not match pattern %q", name, pattern)
	}
	var id string
	if err := resourcename.Sscan(name, pattern, &id); err != nil {
		return "", status.Errorf(codes.InvalidArgument, "failed to scan resource name %q: %v", name, err)
	}
	return id, nil
}

// ParseChildResourceIDs extracts `{parent_id}` and `{child_id}` from a nested AIP resource name like `zfsPools/{zfs_pool}/datasets/{dataset}`.
func ParseChildResourceIDs(pattern, name string) (string, string, error) {
	if err := resourcename.Validate(name); err != nil {
		return "", "", status.Errorf(codes.InvalidArgument, "invalid resource name %q: %v", name, err)
	}
	if !resourcename.Match(pattern, name) {
		return "", "", status.Errorf(codes.InvalidArgument, "resource name %q does not match pattern %q", name, pattern)
	}
	var parentID, childID string
	if err := resourcename.Sscan(name, pattern, &parentID, &childID); err != nil {
		return "", "", status.Errorf(codes.InvalidArgument, "failed to scan resource name %q: %v", name, err)
	}
	return parentID, childID, nil
}

// PaginateSlice applies AIP-158 pagination (`page_size` & `page_token`) to an in-memory slice.
func PaginateSlice[T any](items []T, pageSize int32, pageToken string) ([]T, string, int32, error) {
	total := int32(len(items))
	offset := 0
	if pageToken != "" {
		decoded, err := base64.StdEncoding.DecodeString(pageToken)
		if err != nil {
			return nil, "", 0, status.Errorf(codes.InvalidArgument, "invalid page_token")
		}
		off, err := strconv.Atoi(string(decoded))
		if err != nil || off < 0 {
			return nil, "", 0, status.Errorf(codes.InvalidArgument, "invalid page_token offset")
		}
		offset = off
	}
	if offset >= len(items) {
		return []T{}, "", total, nil
	}
	limit := int(pageSize)
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	end := offset + limit
	nextToken := ""
	if end < len(items) {
		nextToken = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", end)))
	} else {
		end = len(items)
	}
	return items[offset:end], nextToken, total, nil
}

// ToGRPCError translates domain errors to AIP-193 canonical gRPC status codes.
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
