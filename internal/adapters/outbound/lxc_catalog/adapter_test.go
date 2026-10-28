package lxc_catalog_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestLxcCatalogAdapter_PersistentDiskCacheAndOnDemandSync(t *testing.T) {
	tempDir := t.TempDir()

	// Mock server for TurnKey aplinfo
	turnkeyMock := `Package: turnkey-mock-app
Version: 18.0-1
OS: debian-12
Architecture: amd64
Location: debian-12-turnkey-mock-app_18.0-1_amd64.tar.gz
Infopage: https://www.turnkeylinux.org/mock-app
Description: Mock appliance for disk caching test
`
	tsTurnkey := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, turnkeyMock)
	}))
	defer tsTurnkey.Close()

	// Mock server for LinuxContainers index-system
	lxcMock := `testos;testrel;amd64;default;20261028_00:00;/images/testos/testrel/amd64/default/20261028_00:00/
`
	tsLxc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, lxcMock)
	}))
	defer tsLxc.Close()

	// 1. Adapter 1: runs on-demand sync and persists to cache directory
	adapter1 := lxc_catalog.NewAdapter(tempDir)
	adapter1.SetURLsForTest("http://127.0.0.1:0/unreachable", tsTurnkey.URL, tsLxc.URL)

	ctx := context.Background()
	count1, syncTime1, err := adapter1.SyncCatalog(ctx, true)
	if err != nil {
		t.Fatalf("SyncCatalog failed: %v", err)
	}
	if count1 == 0 || syncTime1.IsZero() {
		t.Fatalf("expected non-zero count and syncTime: count=%d, time=%v", count1, syncTime1)
	}

	// Verify items from sync are present in adapter1
	_, err = adapter1.GetItem(ctx, "turnkey-mock-app")
	if err != nil {
		t.Fatalf("turnkey-mock-app not found in adapter1: %v", err)
	}
	_, err = adapter1.GetItem(ctx, "lxc-testos-testrel")
	if err != nil {
		t.Fatalf("lxc-testos-testrel not found in adapter1: %v", err)
	}

	// 2. Adapter 2: created against the same directory, starts without network
	adapter2 := lxc_catalog.NewAdapter(tempDir)
	adapter2.SetURLsForTest("http://127.0.0.1:0/unreachable", "http://127.0.0.1:0/unreachable", "http://127.0.0.1:0/unreachable")

	syncTime2 := adapter2.LastSyncTime()
	if syncTime2.IsZero() {
		t.Fatalf("expected adapter2 to restore lastSyncTime from cache, got zero")
	}
	if syncTime2.Unix() != syncTime1.Unix() {
		t.Errorf("expected lastSync time to match: %v vs %v", syncTime2, syncTime1)
	}

	// Verify items loaded from disk cache
	item2, err := adapter2.GetItem(ctx, "turnkey-mock-app")
	if err != nil {
		t.Fatalf("turnkey-mock-app not found in cached adapter2: %v", err)
	}
	if item2.Title != "Mock App" {
		t.Errorf("unexpected item title: %s", item2.Title)
	}

	// 3. On-demand sync with force=false returns cached results without error
	count3, syncTime3, err := adapter2.SyncCatalog(ctx, false)
	if err != nil {
		t.Fatalf("SyncCatalog(force=false) failed on cached catalog: %v", err)
	}
	if count3 != count1 {
		t.Errorf("expected count3 == count1 (%d vs %d)", count3, count1)
	}
	if syncTime3.Unix() != syncTime1.Unix() {
		t.Errorf("expected syncTime %v, got %v", syncTime1, syncTime3)
	}
}
