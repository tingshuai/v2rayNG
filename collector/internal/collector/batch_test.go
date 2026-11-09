package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishAtomicallyKeepsPreviousOutputAsRollback(t *testing.T) {
	workDir := t.TempDir()
	if err := publishAtomically(workDir, []string{"one"}); err != nil {
		t.Fatal(err)
	}
	if err := publishAtomically(workDir, []string{"two"}); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(filepath.Join(workDir, "published", "subscription.previous.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(previous) != "b25l" {
		t.Fatalf("previous output = %q, want base64(one)", previous)
	}
}
