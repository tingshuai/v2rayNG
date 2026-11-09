package collector

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestThirdFailedRunPausesSourceAndExplicitEnableRestoresIt(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	registration := SourceRegistration{ID: "source-one", FetchURL: "https://source.example/subscription", MinFetchInterval: time.Minute}
	if err := store.RegisterSource(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	for range FailurePauseLimit {
		source, err := store.RecordRun(context.Background(), registration.ID, RunFailed, 0, "timeout")
		if err != nil {
			t.Fatal(err)
		}
		if source.ConsecutiveFailures < FailurePauseLimit && !source.Enabled {
			t.Fatal("source paused before failure threshold")
		}
	}
	sources, err := store.EnabledSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("enabled sources = %d, want 0", len(sources))
	}
	if err := store.EnableSource(context.Background(), registration.ID, "operator_request"); err != nil {
		t.Fatal(err)
	}
	sources, err = store.EnabledSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("enabled sources after explicit enable = %d, want 1", len(sources))
	}
}
