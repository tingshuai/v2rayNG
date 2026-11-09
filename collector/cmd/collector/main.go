package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tingshuai/FreeV2ray/collector/internal/collector"
)

func main() {
	if len(os.Args) < 2 {
		fail(errors.New("usage: collector <migrate|source|run|serve>"))
	}
	switch os.Args[1] {
	case "migrate":
		migrate(os.Args[2:])
	case "source":
		source(os.Args[2:])
	case "run":
		run(os.Args[2:])
	case "serve":
		serve(os.Args[2:])
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func run(arguments []string) {
	flags := flag.NewFlagSet("collector run", flag.ExitOnError)
	database := flags.String("database", "./data/collector.sqlite", "SQLite database path")
	migrations := flags.String("migrations", "./migrations", "migration directory")
	workDir := flags.String("work-dir", "./data", "runtime work directory")
	keyFile := flags.String("fingerprint-key-file", "", "path to a 32-byte deployment secret")
	singBoxPath := flags.String("sing-box-path", "/usr/local/bin/sing-box", "path to the restricted sing-box binary")
	probeURL := flags.String("probe-url", "", "unauthenticated HTTPS endpoint expected to return HTTP 204")
	batchDeadline := flags.Duration("batch-deadline", 0, "optional bounded duration for one batch run")
	flags.Parse(arguments)
	if *keyFile == "" {
		fail(errors.New("-fingerprint-key-file is required"))
	}
	key, err := os.ReadFile(*keyFile)
	if err != nil {
		fail(fmt.Errorf("read fingerprint key: %w", err))
	}
	store := openAndMigrate(*database, *migrations)
	defer store.Close()
	fetcher := collector.NewSafeFetcher(nil)
	runner := collector.BatchRunner{Store: store, Fetcher: fetcher, WorkDir: *workDir, FingerprintKey: key, FastDeadline: *batchDeadline, Prober: collector.SingBoxProber{BinaryPath: *singBoxPath, ProbeURL: *probeURL, Resolver: fetcher.Resolver, WorkDir: *workDir}}
	if err := runner.Run(context.Background()); err != nil {
		fail(err)
	}
	slog.Info("collector batch completed")
}

func databaseAndMigrations(arguments []string) (*flag.FlagSet, *string, *string) {
	flags := flag.NewFlagSet("collector", flag.ExitOnError)
	database := flags.String("database", "./data/collector.sqlite", "SQLite database path")
	migrations := flags.String("migrations", "./migrations", "migration directory")
	flags.Parse(arguments)
	return flags, database, migrations
}

func migrate(arguments []string) {
	_, database, migrations := databaseAndMigrations(arguments)
	store := openAndMigrate(*database, *migrations)
	defer store.Close()
	slog.Info("collector migrations applied")
}

func source(arguments []string) {
	if len(arguments) < 1 {
		fail(errors.New("usage: collector source <add|update|pause|enable|delete>"))
	}
	action := arguments[0]
	flags := flag.NewFlagSet("collector source "+action, flag.ExitOnError)
	database := flags.String("database", "./data/collector.sqlite", "SQLite database path")
	migrations := flags.String("migrations", "./migrations", "migration directory")
	id := flags.String("id", "", "stable source ID")
	url := flags.String("url", "", "unauthenticated HTTPS source URL")
	interval := flags.Duration("min-fetch-interval", 15*time.Minute, "minimum interval between source fetches")
	reason := flags.String("reason", "operator_request", "non-secret state-change reason")
	flags.Parse(arguments[1:])
	store := openAndMigrate(*database, *migrations)
	defer store.Close()
	ctx := context.Background()
	switch action {
	case "add":
		if err := store.RegisterSource(ctx, collector.SourceRegistration{ID: *id, FetchURL: *url, MinFetchInterval: *interval}); err != nil {
			fail(err)
		}
		slog.Info("source registered", "source_id", *id)
	case "update":
		if err := store.UpdateSource(ctx, collector.SourceRegistration{ID: *id, FetchURL: *url, MinFetchInterval: *interval}, *reason); err != nil {
			fail(err)
		}
		slog.Info("source updated", "source_id", *id)
	case "pause":
		if err := store.PauseSource(ctx, *id, *reason); err != nil {
			fail(err)
		}
		slog.Info("source paused", "source_id", *id)
	case "enable":
		if err := store.EnableSource(ctx, *id, *reason); err != nil {
			fail(err)
		}
		slog.Info("source enabled", "source_id", *id)
	case "delete":
		if err := store.DeleteSource(ctx, *id, *reason); err != nil {
			fail(err)
		}
		slog.Info("source deleted", "source_id", *id)
	default:
		fail(fmt.Errorf("unknown source action %q", action))
	}
}

func serve(arguments []string) {
	flags := flag.NewFlagSet("collector serve", flag.ExitOnError)
	database := flags.String("database", "./data/collector.sqlite", "SQLite database path")
	migrations := flags.String("migrations", "./migrations", "migration directory")
	listen := flags.String("listen", "127.0.0.1:8080", "loopback listen address")
	workDir := flags.String("work-dir", "./data", "runtime work directory")
	flags.Parse(arguments)
	store := openAndMigrate(*database, *migrations)
	defer store.Close()
	server := &http.Server{Addr: *listen, Handler: newServeMux(*workDir), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	slog.Info("collector listening", "address", *listen)
	fail(server.ListenAndServe())
}

func newServeMux(workDir string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /subscription", func(writer http.ResponseWriter, request *http.Request) {
		output, err := os.ReadFile(filepath.Join(workDir, "published", "subscription.txt"))
		if errors.Is(err, os.ErrNotExist) {
			http.Error(writer, "subscription unavailable", http.StatusServiceUnavailable)
			return
		}
		if err != nil {
			slog.Error("read published subscription", "component", "http", "error", err)
			http.Error(writer, "subscription unavailable", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = writer.Write(output)
	})
	return mux
}

func openAndMigrate(database, migrations string) *collector.SQLiteStore {
	if err := os.MkdirAll(filepath.Dir(database), 0o700); err != nil {
		fail(fmt.Errorf("create database directory: %w", err))
	}
	store, err := collector.OpenSQLite(database)
	if err != nil {
		fail(err)
	}
	if err := store.Migrate(context.Background(), migrations); err != nil {
		store.Close()
		fail(err)
	}
	return store
}

func fail(err error) {
	if errors.Is(err, http.ErrServerClosed) {
		return
	}
	slog.Error("collector stopped", "error", err)
	os.Exit(1)
}
