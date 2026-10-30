package grpcadapter

import (
	"context"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TokenVerifier resolves a bearer token into the authenticated Linux user.
type TokenVerifier interface {
	VerifyToken(ctx context.Context, token string) (*domain.User, error)
}

// publicMethods can be invoked without a bearer token.
var publicMethods = map[string]bool{
	nasv1.NasService_Login_FullMethodName: true,
}

type callerKey struct{}

// CallerFromContext returns the user authenticated by the auth interceptor, if any.
func CallerFromContext(ctx context.Context) *domain.User {
	u, _ := ctx.Value(callerKey{}).(*domain.User)
	return u
}

// WithCaller returns a child context carrying the authenticated domain.User.
func WithCaller(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, callerKey{}, u)
}

// ServerOptions returns the interceptors every NasService gRPC server must run:
// bearer-token authentication followed by request ID validation.
func ServerOptions(verifier TokenVerifier) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			ctx, err := authenticate(ctx, verifier, info.FullMethod)
			if err != nil {
				return nil, err
			}
			if msg, ok := req.(proto.Message); ok {
				if err := ValidateRequestIDs(msg); err != nil {
					return nil, err
				}
			}
			return handler(ctx, req)
		}),
		grpc.ChainStreamInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx, err := authenticate(ss.Context(), verifier, info.FullMethod)
			if err != nil {
				return err
			}
			return handler(srv, &authedStream{ServerStream: ss, ctx: ctx})
		}),
	}
}

type authedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authedStream) Context() context.Context { return s.ctx }

func (s *authedStream) RecvMsg(m any) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}
	if msg, ok := m.(proto.Message); ok {
		return ValidateRequestIDs(msg)
	}
	return nil
}

func authenticate(ctx context.Context, verifier TokenVerifier, method string) (context.Context, error) {
	if publicMethods[method] {
		return ctx, nil
	}
	if verifier == nil {
		return nil, status.Error(codes.Unauthenticated, "authentication is not configured")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	vals := md.Get("authorization")
	if len(vals) == 0 || strings.TrimSpace(vals[0]) == "" {
		return nil, status.Error(codes.Unauthenticated, "missing bearer token")
	}
	user, err := verifier.VerifyToken(ctx, vals[0])
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
	}
	return context.WithValue(ctx, callerKey{}, user), nil
}

// resourcePathFields hold '/'-separated identifiers rather than single IDs.
var resourcePathFields = map[string]bool{
	"name":           true,
	"parent":         true,
	"zfs_dataset_id": true,
	"zfs_volume_id":  true,
}

// extraIDFields are identifier fields that do not follow the *_id naming convention.
var extraIDFields = map[string]bool{
	"pool_name":     true,
	"target_pool":   true,
	"service_name":  true,
	"snapshot_name": true,
}

// ValidateRequestIDs rejects requests whose identifier fields contain anything
// outside [a-zA-Z0-9._-] (per '/'-separated segment). It checks the request's own
// string fields and the `name`/`pool_name` of an embedded resource, since create
// handlers fall back to those when no explicit ID is supplied.
func ValidateRequestIDs(msg proto.Message) error {
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.IsList() || fd.IsMap() {
			continue
		}
		switch fd.Kind() {
		case protoreflect.StringKind:
			if err := validateIDField(string(fd.Name()), m.Get(fd).String()); err != nil {
				return err
			}
		case protoreflect.MessageKind:
			if !m.Has(fd) {
				continue
			}
			nested := m.Get(fd).Message()
			for _, name := range []protoreflect.Name{"name", "pool_name"} {
				nfd := nested.Descriptor().Fields().ByName(name)
				if nfd == nil || nfd.Kind() != protoreflect.StringKind || nfd.IsList() {
					continue
				}
				if err := validateIDField(string(fd.Name())+"."+string(name), nested.Get(nfd).String()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateIDField(field, value string) error {
	if value == "" {
		return nil
	}
	short := field[strings.LastIndex(field, ".")+1:]
	var err error
	switch {
	case resourcePathFields[short]:
		err = domain.ValidateResourcePath(value, field)
	case short == "id" || strings.HasSuffix(short, "_id") || extraIDFields[short]:
		err = domain.ValidateResourceID(value, field)
	}
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return nil
}
