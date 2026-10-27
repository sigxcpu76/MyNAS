package identity

import (
	"context"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestIdentityAdapter_UID_GID_ValidationAndFiltering(t *testing.T) {
	ctx := context.Background()
	adapter := NewAdapter()

	// 1. Verify ListUsers and ListGroups filter out < 1000 and > 10000 by default
	users, err := adapter.ListUsers(ctx, false)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	for _, u := range users {
		if u.UID < 1000 || u.UID > 10000 {
			t.Errorf("regular user %s has unexpected UID %d (must be 1000..10000)", u.Username, u.UID)
		}
	}

	groups, err := adapter.ListGroups(ctx, false)
	if err != nil {
		t.Fatalf("ListGroups failed: %v", err)
	}
	for _, g := range groups {
		if g.GID < 1000 || g.GID > 10000 {
			t.Errorf("regular group %s has unexpected GID %d (must be 1000..10000)", g.GroupName, g.GID)
		}
	}

	// 2. Test auto-assignment of UID in 1000..10000
	user1, err := adapter.CreateUser(ctx, domain.User{
		ID:       "testuser1",
		Username: "testuser1",
	})
	if err != nil {
		t.Fatalf("CreateUser testuser1 failed: %v", err)
	}
	if user1.UID < 1000 || user1.UID > 10000 {
		t.Errorf("expected auto-assigned UID in 1000..10000, got %d", user1.UID)
	}

	// 3. Test rejection of duplicate UID
	_, err = adapter.CreateUser(ctx, domain.User{
		ID:       "testuser2",
		Username: "testuser2",
		UID:      user1.UID,
	})
	if err == nil {
		t.Errorf("expected error when creating user with duplicate UID %d, got nil", user1.UID)
	}

	// 4. Test auto-assignment of GID in 1000..10000
	group1, err := adapter.CreateGroup(ctx, domain.Group{
		ID:        "testgroup1",
		GroupName: "testgroup1",
	})
	if err != nil {
		t.Fatalf("CreateGroup testgroup1 failed: %v", err)
	}
	if group1.GID < 1000 || group1.GID > 10000 {
		t.Errorf("expected auto-assigned GID in 1000..10000, got %d", group1.GID)
	}

	// 5. Test rejection of duplicate GID
	_, err = adapter.CreateGroup(ctx, domain.Group{
		ID:        "testgroup2",
		GroupName: "testgroup2",
		GID:       group1.GID,
	})
	if err == nil {
		t.Errorf("expected error when creating group with duplicate GID %d, got nil", group1.GID)
	}

	// 6. Test update rejection of duplicate GID
	group2, err := adapter.CreateGroup(ctx, domain.Group{
		ID:        "testgroup2",
		GroupName: "testgroup2",
	})
	if err != nil {
		t.Fatalf("CreateGroup testgroup2 failed: %v", err)
	}
	_, err = adapter.UpdateGroup(ctx, domain.Group{
		ID:  group2.ID,
		GID: group1.GID,
	}, []string{"gid"})
	if err == nil {
		t.Errorf("expected error updating group2 to existing GID %d, got nil", group1.GID)
	}
}
