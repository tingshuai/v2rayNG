package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tingshuai/FreeV2ray/collector/internal/collector"
)

func main() {
	if len(os.Args) < 2 {
		fail(errors.New("usage: collector <migrate|source|candidate|run|serve>"))
	}
	switch os.Args[1] {
	case "migrate":
		migrate(os.Args[2:])
	case "source":
		source(os.Args[2:])
	case "candidate":
		candidate(os.Args[2:])
	case "run":
		run(os.Args[2:])
	case "serve":
		serve(os.Args[2:])
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func candidate(arguments []string) {
	if len(arguments) == 0 || (arguments[0] != "revoke" && arguments[0] != "restore") {
		fail(errors.New("usage: collector candidate <revoke|restore> -id <manifest-id> -reason <allowed-code>"))
	}
	action := arguments[0]
	flags := flag.NewFlagSet("collector candidate "+action, flag.ExitOnError)
	database := flags.String("database", "./data/collector.sqlite", "SQLite database path")
	migrations := flags.String("migrations", "./migrations", "migration directory")
	id := flags.String("id", "", "opaque candidate ID from manifest")
	reason := flags.String("reason", "", "allowed non-secret reason code")
	flags.Parse(arguments[1:])
	store := openAndMigrate(*database, *migrations)
	defer store.Close()
	if action == "revoke" {
		if err := store.RevokeCandidate(context.Background(), *id, *reason); err != nil {
			fail(err)
		}
	} else if err := store.RestoreCandidate(context.Background(), *id, *reason); err != nil {
		fail(err)
	}
	slog.Info("candidate revocation state changed", "candidate_id", *id, "action", action, "reason", *reason)
}

func run(arguments []string) {
	flags := flag.NewFlagSet("collector run", flag.ExitOnError)
	database := flags.String("database", "./data/collector.sqlite", "SQLite database path")
	migrations := flags.String("migrations", "./migrations", "migration directory")
	workDir := flags.String("work-dir", "./data", "runtime work directory")
	keyFile := flags.String("fingerprint-key-file", "", "path to a 32-byte deployment secret")
	singBoxPath := flags.String("sing-box-path", "/usr/local/bin/sing-box", "path to the restricted sing-box binary")
	probeURL := flags.String("probe-url", "", "unauthenticated HTTPS endpoint expected to return HTTP 204")
	batchDeadline := flags.Duration("batch-deadline", 0, "optional bounded duration for source screening; finalization gets a separate short budget")
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
	runner := collector.BatchRunner{Store: store, Fetcher: fetcher, Resolver: fetcher.Resolver, WorkDir: *workDir, FingerprintKey: key, FastDeadline: *batchDeadline, Prober: collector.SingBoxProber{BinaryPath: *singBoxPath, ProbeURL: *probeURL, Resolver: fetcher.Resolver, WorkDir: *workDir}}
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
	maxPublishAge := flags.Duration("max-publish-age", 2*time.Hour, "maximum age of a published subscription accepted by /healthz")
	keyFile := flags.String("fingerprint-key-file", "", "path to a 32-byte deployment secret used for anonymous distribution")
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
	selector := func(ctx context.Context, published []byte) ([]byte, error) {
		return collector.DistributeSubscription(ctx, store, key, published, collector.DefaultDistributionLimit)
	}
	manifestSelector := func(ctx context.Context, published []byte) ([]byte, error) {
		info, err := os.Stat(filepath.Join(*workDir, "published", "subscription.txt"))
		if err != nil {
			return nil, err
		}
		return collector.BuildCandidateManifest(ctx, store, key, published, collector.DefaultDistributionLimit, info.ModTime(), time.Now().UTC())
	}
	server := &http.Server{Addr: *listen, Handler: newServeMuxWithManifest(*workDir, *maxPublishAge, selector, manifestSelector, store), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	slog.Info("collector listening", "address", *listen)
	fail(server.ListenAndServe())
}

type subscriptionSelector func(context.Context, []byte) ([]byte, error)
type manifestSelector func(context.Context, []byte) ([]byte, error)

func newServeMux(workDir string, maxPublishAge time.Duration, selectors ...subscriptionSelector) *http.ServeMux {
	return newServeMuxInternal(workDir, maxPublishAge, selectors, nil, nil)
}

func newServeMuxWithManifest(workDir string, maxPublishAge time.Duration, selector subscriptionSelector, manifest manifestSelector, feedback feedbackRecorder) *http.ServeMux {
	return newServeMuxInternal(workDir, maxPublishAge, []subscriptionSelector{selector}, &manifest, feedback)
}

func newServeMuxInternal(workDir string, maxPublishAge time.Duration, selectors []subscriptionSelector, manifest *manifestSelector, feedback feedbackRecorder) *http.ServeMux {
	mux := http.NewServeMux()
	feedbackLimiter := &windowRateLimiter{limit: 120, window: time.Minute}
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		info, err := os.Stat(filepath.Join(workDir, "published", "subscription.txt"))
		if err != nil || time.Since(info.ModTime()) > maxPublishAge {
			http.Error(writer, "subscription is missing or stale", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /subscription", func(writer http.ResponseWriter, request *http.Request) {
		output, status := readPublished(writer, workDir)
		if status != 0 {
			return
		}
		response := output
		if len(selectors) > 0 {
			selected, err := selectors[0](request.Context(), output)
			if err != nil {
				slog.Error("select subscription candidates", "component", "http", "error", err)
			} else {
				response = selected
			}
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		_, _ = writer.Write(response)
	})
	if manifest != nil {
		mux.HandleFunc("GET /manifest", func(writer http.ResponseWriter, request *http.Request) {
			output, status := readPublished(writer, workDir)
			if status != 0 {
				return
			}
			response, err := (*manifest)(request.Context(), output)
			if err != nil {
				slog.Error("build candidate manifest", "component", "http", "error", err)
				http.Error(writer, "manifest unavailable", http.StatusInternalServerError)
				return
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			writer.Header().Set("Cache-Control", "no-store")
			_, _ = writer.Write(response)
		})
	}
	mux.HandleFunc("POST /feedback", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		if !feedbackLimiter.allow(time.Now()) {
			http.Error(writer, "feedback rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, 2048)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		var reports []collector.FeedbackReport
		if err := decoder.Decode(&reports); err != nil || len(reports) == 0 || len(reports) > 8 {
			http.Error(writer, "invalid feedback", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			http.Error(writer, "invalid feedback", http.StatusBadRequest)
			return
		}
		for _, report := range reports {
			if report.CandidateID != strings.ToLower(report.CandidateID) || report.Validate() != nil {
				http.Error(writer, "invalid feedback", http.StatusBadRequest)
				return
			}
		}
		if feedback == nil {
			http.Error(writer, "feedback unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := feedback.RecordFeedbackBatch(request.Context(), reports, time.Now().UTC()); err != nil {
			slog.Error("aggregate candidate feedback", "component", "http", "error", err)
			http.Error(writer, "feedback unavailable", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	return mux
}

type feedbackRecorder interface {
	RecordFeedbackBatch(context.Context, []collector.FeedbackReport, time.Time) error
}

type windowRateLimiter struct {
	mutex   sync.Mutex
	limit   int
	window  time.Duration
	started time.Time
	count   int
}

func (limiter *windowRateLimiter) allow(now time.Time) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	if limiter.started.IsZero() || now.Sub(limiter.started) >= limiter.window {
		limiter.started = now
		limiter.count = 0
	}
	if limiter.count >= limiter.limit {
		return false
	}
	limiter.count++
	return true
}

func readPublished(writer http.ResponseWriter, workDir string) ([]byte, int) {
	output, err := os.ReadFile(filepath.Join(workDir, "published", "subscription.txt"))
	if errors.Is(err, os.ErrNotExist) {
		http.Error(writer, "subscription unavailable", http.StatusServiceUnavailable)
		return nil, http.StatusServiceUnavailable
	}
	if err != nil {
		slog.Error("read published subscription", "component", "http", "error", err)
		http.Error(writer, "subscription unavailable", http.StatusInternalServerError)
		return nil, http.StatusInternalServerError
	}
	return output, 0
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
