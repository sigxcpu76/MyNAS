package lxc_catalog_test

import (
	"context"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc_catalog"
)

func TestLxcCatalogAdapter_SeedAndFilter(t *testing.T) {
	ctx := context.Background()
	adapter := lxc_catalog.NewAdapter(t.TempDir())

	// 1. Check all seeded items
	allItems, err := adapter.ListItems(ctx, "all", "", "")
	if err != nil {
		t.Fatalf("ListItems failed: %v", err)
	}
	if len(allItems) < 20 {
		t.Fatalf("expected at least 20 seeded catalog items, got %d", len(allItems))
	}

	// 2. Filter by Community-Scripts source
	commItems, err := adapter.ListItems(ctx, "community-scripts", "", "")
	if err != nil {
		t.Fatalf("ListItems for community-scripts failed: %v", err)
	}
	if len(commItems) == 0 {
		t.Fatalf("expected community-scripts items, got 0")
	}
	for _, it := range commItems {
		if it.Source != "community-scripts" {
			t.Fatalf("expected source community-scripts, got %s", it.Source)
		}
	}

	// 3. Filter by TurnKey source
	turnkeyItems, err := adapter.ListItems(ctx, "turnkey", "", "")
	if err != nil {
		t.Fatalf("ListItems for turnkey failed: %v", err)
	}
	if len(turnkeyItems) == 0 {
		t.Fatalf("expected turnkey items, got 0")
	}
	for _, it := range turnkeyItems {
		if it.Source != "turnkey" {
			t.Fatalf("expected source turnkey, got %s", it.Source)
		}
		if it.DownloadURL == "" {
			t.Fatalf("expected non-empty download URL for TurnKey appliance: %+v", it)
		}
	}

	// 4. Filter by LinuxContainers source
	lxcItems, err := adapter.ListItems(ctx, "linuxcontainers", "", "")
	if err != nil {
		t.Fatalf("ListItems for linuxcontainers failed: %v", err)
	}
	if len(lxcItems) == 0 {
		t.Fatalf("expected linuxcontainers items, got 0")
	}
	for _, it := range lxcItems {
		if it.Source != "linuxcontainers" {
			t.Fatalf("expected source linuxcontainers, got %s", it.Source)
		}
		if it.DownloadURL == "" {
			t.Fatalf("expected non-empty download URL for LinuxContainers base image: %+v", it)
		}
	}

	// 5. Search query
	searchItems, err := adapter.ListItems(ctx, "all", "", "pi-hole")
	if err != nil {
		t.Fatalf("ListItems search failed: %v", err)
	}
	if len(searchItems) == 0 || searchItems[0].ID != "community-pihole" {
		t.Fatalf("expected to find community-pihole, got: %+v", searchItems)
	}

	// 6. Get item by ID
	item, err := adapter.GetItem(ctx, "community-plex")
	if err != nil {
		t.Fatalf("GetItem community-plex failed: %v", err)
	}
	if item.Title != "Plex Media Server" || !item.GpuRecommended || item.DefaultPort != 32400 {
		t.Fatalf("unexpected item values: %+v", item)
	}
}
