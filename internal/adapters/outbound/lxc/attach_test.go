package lxc

import (
	"slices"
	"testing"
)

func TestBuildLxcAttachArgs(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		shell    string
		wantTail []string
	}{
		{
			name:     "bash uses login flag",
			id:       "ct-100",
			shell:    "/bin/bash",
			wantTail: []string{"--", "/bin/bash", "-l"},
		},
		{
			name:     "sh uses login flag",
			id:       "ct-101",
			shell:    "/bin/sh",
			wantTail: []string{"--", "/bin/sh", "-l"},
		},
		{
			name:     "ash uses login flag",
			id:       "ct-102",
			shell:    "/bin/ash",
			wantTail: []string{"--", "/bin/ash", "-l"},
		},
		{
			name:     "custom binary omits login flag",
			id:       "ct-103",
			shell:    "/usr/bin/custom-cli",
			wantTail: []string{"--", "/usr/bin/custom-cli"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildLxcAttachArgs(tc.id, tc.shell)

			wantPrefix := []string{
				"-n", tc.id,
				"-v", "TERM=xterm-256color",
				"-v", "COLORTERM=truecolor",
				"-v", "LANG=C.UTF-8",
				"-v", "LC_CTYPE=C.UTF-8",
				"-v", "HOME=/root",
			}
			want := append(wantPrefix, tc.wantTail...)
			if !slices.Equal(got, want) {
				t.Fatalf("buildLxcAttachArgs(%q, %q) = %v, want %v", tc.id, tc.shell, got, want)
			}
		})
	}
}
