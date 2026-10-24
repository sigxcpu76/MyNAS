package prereq_test

import (
	"context"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestPrereqChecker(t *testing.T) {
	ctx := context.Background()
	checker := prereq.NewSystemChecker(true)

	results, fallback, err := checker.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected prerequisites to be checked")
	}

	for _, p := range results {
		t.Logf("Prereq %s (%s): %s [v: %s] fallback=%v (details: %s)",
			p.ID, p.Category, p.Status, p.Version, p.FallbackMode, p.Details)
		if p.ID == "zfs-module" || p.ID == "libzfs" {
			if p.Status == domain.StatusInstalled {
				t.Logf("Verified ZFS kernel/lib loaded: %s", p.Version)
			}
		}
	}

	_ = fallback
}
