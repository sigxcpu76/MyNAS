package prereq_test

import (
	"context"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestPrereqChecker(t *testing.T) {
	ctx := context.Background()
	checker := prereq.NewSystemChecker()

	results, err := checker.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected prerequisites to be checked")
	}

	for _, p := range results {
		t.Logf("Prereq %s (%s): %s [v: %s] (details: %s)",
			p.ID, p.Category, p.Status, p.Version, p.Details)
		if p.ID == "zfs-module" || p.ID == "libzfs" {
			if p.Status == domain.StatusInstalled {
				t.Logf("Verified ZFS kernel/lib loaded: %s", p.Version)
			}
		}
	}
}

func TestPrerequisitesReport(t *testing.T) {
	ctx := context.Background()
	checker := prereq.NewSystemChecker()

	report, err := checker.GetReport(ctx)
	if err != nil {
		t.Fatalf("GetReport failed: %v", err)
	}

	if report.LinuxFlavor.ID == "" {
		t.Fatalf("expected detected Linux flavor ID to be non-empty")
	}
	if report.LinuxFlavor.PackageManager == "" {
		t.Fatalf("expected detected package manager to be non-empty")
	}

	t.Logf("Detected Linux Flavor: %s (ID: %s, PM: %s)",
		report.LinuxFlavor.PrettyName, report.LinuxFlavor.ID, report.LinuxFlavor.PackageManager)

	t.Logf("Installed: %d, Missing: %d, Fully Compliant: %v",
		len(report.Installed), len(report.Missing), report.IsFullyCompliant)

	if len(report.Missing) > 0 {
		if report.RemediationScript == "" {
			t.Fatalf("expected remediation script when missing prerequisites exist")
		}
		t.Logf("Remediation script:\n%s", report.RemediationScript)
	}

	// Verify all installed items have status == INSTALLED
	for _, p := range report.Installed {
		if p.Status != domain.StatusInstalled {
			t.Errorf("expected installed prereq %s to have StatusInstalled, got %s", p.ID, p.Status)
		}
	}

	// Verify all missing items have status != INSTALLED
	for _, p := range report.Missing {
		if p.Status == domain.StatusInstalled {
			t.Errorf("expected missing prereq %s to not have StatusInstalled", p.ID)
		}
		if p.InstallHint == "" {
			t.Errorf("expected install hint for missing prereq %s", p.ID)
		}
	}
}
