package grpcadapter_test

import (
	"testing"

	grpcadapter "github.com/sigxcpu/nas/internal/adapters/inbound/grpc"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestValidateRequestIDs(t *testing.T) {
	valid := []proto.Message{
		&nasv1.CreateDockerAppRequest{DockerAppId: "nextcloud", DockerApp: &nasv1.DockerApp{}},
		&nasv1.CreateZfsDatasetRequest{Parent: "zfsPools/tank", ZfsDatasetId: "media/movies_4k"},
		&nasv1.GetZfsDatasetRequest{Name: "zfsPools/tank/datasets/media"},
		&nasv1.UpdateUserRequest{User: &nasv1.User{Name: "users/alice"}},
		&nasv1.CreateZfsPoolRequest{ZfsPool: &nasv1.ZfsPool{PoolName: "tank"}},
		&nasv1.LoginRequest{Username: "alice", Password: "any\nthing"},
	}
	for _, msg := range valid {
		if err := grpcadapter.ValidateRequestIDs(msg); err != nil {
			t.Errorf("%T: unexpected error: %v", msg, err)
		}
	}

	invalid := []proto.Message{
		&nasv1.CreateDockerAppRequest{DockerAppId: ".."},
		&nasv1.CreateDockerAppRequest{DockerApp: &nasv1.DockerApp{Name: "dockerApps/.."}},
		&nasv1.CreateLxcContainerRequest{LxcContainerId: "a\nlxc.hook.pre-start=/bin/sh"},
		&nasv1.CreateZfsDatasetRequest{Parent: "zfsPools/tank", ZfsDatasetId: "media//x"},
		&nasv1.CreateZfsPoolRequest{ZfsPool: &nasv1.ZfsPool{PoolName: "-f"}},
		&nasv1.ImportZfsPoolRequest{PoolName: "-a"},
		&nasv1.CreateVirtualMachineSnapshotRequest{Parent: "virtualMachines/vm1", SnapshotId: "--diskspec"},
		&nasv1.AttachVirtualMachineMediaRequest{Name: "virtualMachines/vm1", DeviceId: "ide2 extra"},
		&nasv1.GetUserRequest{Name: "users/../root"},
	}
	for _, msg := range invalid {
		err := grpcadapter.ValidateRequestIDs(msg)
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("%T %v: expected InvalidArgument, got %v", msg, msg, err)
		}
	}
}
