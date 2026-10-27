package docker

import (
	"errors"
	"strings"
	"testing"
)

func TestFilterComposeOutputDropsProgress(t *testing.T) {
	raw := "npm Pulling \n 6b37362b3da7 Pulling fs layer \n 6b37362b3da7 Downloading [==>   ]  1MB/2MB\n" +
		" 6b37362b3da7 Extracting [=>  ]  1MB/2MB\n 6b37362b3da7 Pull complete \n Container x-web-1 Creating \n" +
		"Error response from daemon: Conflict. The container name is in use\ncanceled"
	got := filterComposeOutput(raw)
	if strings.Contains(got, "Downloading") || strings.Contains(got, "Pulling") || strings.Contains(got, "Creating") {
		t.Fatalf("progress noise not filtered: %q", got)
	}
	if !strings.Contains(got, "Error response from daemon") {
		t.Fatalf("real error dropped: %q", got)
	}
}

func TestIsComposeCanceled(t *testing.T) {
	if !isComposeCanceled(errors.New("exit status 130"), "") {
		t.Fatal("exit 130 should be treated as canceled")
	}
	if isComposeCanceled(errors.New("exit status 1"), "Error response from daemon") {
		t.Fatal("real failure misclassified as canceled")
	}
}

func TestStripDockerLogHeaders_InvalidUTF8(t *testing.T) {
	// Frame with invalid UTF-8 byte 0xd7 (from torrent names/binary stream)
	raw := []byte{1, 0, 0, 0, 0, 0, 0, 5, 'h', 'e', 0xd7, 'l', 'o'}
	cleaned := stripDockerLogHeaders(raw)
	if !strings.Contains(cleaned, "he") || !strings.Contains(cleaned, "lo") {
		t.Fatalf("unexpected cleaned output: %q", cleaned)
	}
	if len(cleaned) == 0 {
		t.Fatal("cleaned output is empty")
	}
}
