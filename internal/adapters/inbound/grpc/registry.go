package grpcadapter

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
)

func dockerRegistryToProto(r *domain.DockerRegistry) *nasv1.DockerRegistry {
	out := &nasv1.DockerRegistry{
		Name:            "dockerRegistries/" + r.ID,
		Server:          r.Server,
		Username:        r.Username,
		HasPassword:     r.HasPassword,
		Description:     r.Description,
		LastLoginStatus: r.LastLoginStatus,
	}
	if !r.LastLoginTime.IsZero() {
		out.LastLoginTime = r.LastLoginTime.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) requireRegistry() error {
	if s.registry == nil {
		return status.Error(codes.Unavailable, "docker registry service unavailable")
	}
	return s.prereq.RequireModule("docker")
}

func (s *Server) ListDockerRegistries(ctx context.Context, _ *nasv1.ListDockerRegistriesRequest) (*nasv1.ListDockerRegistriesResponse, error) {
	if err := s.requireRegistry(); err != nil {
		return nil, ToGRPCError(err)
	}
	list, err := s.registry.List(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	resp := &nasv1.ListDockerRegistriesResponse{}
	for i := range list {
		resp.DockerRegistries = append(resp.DockerRegistries, dockerRegistryToProto(&list[i]))
	}
	return resp, nil
}

func (s *Server) GetDockerRegistry(ctx context.Context, req *nasv1.GetDockerRegistryRequest) (*nasv1.DockerRegistry, error) {
	if err := s.requireRegistry(); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerRegistry, req.GetName())
	if err != nil {
		return nil, err
	}
	r, err := s.registry.Get(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerRegistryToProto(r), nil
}

func (s *Server) CreateDockerRegistry(ctx context.Context, req *nasv1.CreateDockerRegistryRequest) (*nasv1.DockerRegistry, error) {
	if err := s.requireRegistry(); err != nil {
		return nil, ToGRPCError(err)
	}
	in := req.GetDockerRegistry()
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "docker_registry is required")
	}
	r, err := s.registry.Create(ctx, req.GetDockerRegistryId(), domain.DockerRegistry{
		Server:      in.GetServer(),
		Username:    in.GetUsername(),
		Password:    in.GetPassword(),
		Description: in.GetDescription(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerRegistryToProto(r), nil
}

func (s *Server) UpdateDockerRegistry(ctx context.Context, req *nasv1.UpdateDockerRegistryRequest) (*nasv1.DockerRegistry, error) {
	if err := s.requireRegistry(); err != nil {
		return nil, ToGRPCError(err)
	}
	in := req.GetDockerRegistry()
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "docker_registry is required")
	}
	id, err := ParseSingleResourceID(patternDockerRegistry, in.GetName())
	if err != nil {
		return nil, err
	}
	var paths []string
	if m := req.GetUpdateMask(); m != nil {
		paths = m.GetPaths()
	}
	r, err := s.registry.Update(ctx, domain.DockerRegistry{
		ID:          id,
		Server:      in.GetServer(),
		Username:    in.GetUsername(),
		Password:    in.GetPassword(),
		Description: in.GetDescription(),
	}, paths)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerRegistryToProto(r), nil
}

func (s *Server) DeleteDockerRegistry(ctx context.Context, req *nasv1.DeleteDockerRegistryRequest) (*emptypb.Empty, error) {
	if err := s.requireRegistry(); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerRegistry, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.registry.Delete(ctx, id); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}
