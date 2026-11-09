package collector

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BatchRunner struct {
	Store          Store
	Fetcher        *SafeFetcher
	WorkDir        string
	FingerprintKey []byte
	FastDeadline   time.Duration
	TCPConcurrency int
}

func (runner BatchRunner) Run(ctx context.Context) error {
	if len(runner.FingerprintKey) < 32 {
		return errors.New("fingerprint key must contain at least 32 bytes")
	}
	if runner.FastDeadline <= 0 {
		runner.FastDeadline = 30 * time.Second
	}
	if runner.TCPConcurrency <= 0 {
		runner.TCPConcurrency = 2
	}
	ctx, cancel := context.WithTimeout(ctx, runner.FastDeadline)
	defer cancel()
	sources, err := runner.Store.EnabledSources(ctx)
	if err != nil {
		return err
	}
	var accepted []string
	seen := map[string]struct{}{}
	for _, source := range sources {
		if ctx.Err() != nil {
			break
		}
		candidates, fetchErr := runner.fetchAndScreen(ctx, source)
		if fetchErr != nil {
			_, _ = runner.Store.RecordRun(ctx, source.ID, RunFailed, 0, "fetch_or_screen_failed")
			continue
		}
		if _, err := runner.Store.RecordRun(ctx, source.ID, RunSucceeded, len(candidates), ""); err != nil {
			return err
		}
		for _, candidate := range candidates {
			fingerprint := fingerprint(runner.FingerprintKey, candidate)
			if _, exists := seen[fingerprint]; !exists {
				seen[fingerprint] = struct{}{}
				accepted = append(accepted, candidate)
			}
		}
	}
	if len(accepted) == 0 {
		return errors.New("batch produced no screened candidates; retained previous output")
	}
	sort.Strings(accepted)
	return publishAtomically(runner.WorkDir, accepted)
}

func (runner BatchRunner) fetchAndScreen(ctx context.Context, source Source) ([]string, error) {
	body, err := runner.Fetcher.Fetch(ctx, source)
	if err != nil {
		return nil, err
	}
	candidates, err := splitCandidates(body)
	if err != nil {
		return nil, err
	}
	accepted := runner.screenCandidates(ctx, candidates)
	if len(accepted) == 0 {
		return nil, errors.New("no candidates passed bounded TCP screening")
	}
	return accepted, nil
}

func (runner BatchRunner) screenCandidates(ctx context.Context, candidates []string) []string {
	limit := len(candidates)
	if limit > 50 {
		limit = 50
	}
	candidates = candidates[:limit]
	accepted := make(chan string, limit)
	jobs := make(chan string)
	for range runner.TCPConcurrency {
		go func() {
			for candidate := range jobs {
				endpoint, err := candidateEndpoint(candidate)
				if err == nil {
					probeContext, cancel := context.WithTimeout(ctx, 3*time.Second)
					err = tcpReachable(probeContext, runner.Fetcher.Resolver, endpoint)
					cancel()
					if err == nil {
						select {
						case accepted <- candidate:
						case <-ctx.Done():
						}
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case jobs <- candidate:
			case <-ctx.Done():
				return
			}
		}
	}()
	result := make([]string, 0, limit)
	for range candidates {
		select {
		case candidate := <-accepted:
			result = append(result, candidate)
		case <-ctx.Done():
			return result
		case <-time.After(3 * time.Second):
			return result
		}
	}
	return result
}

func candidateEndpoint(candidate string) (string, error) {
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", err
	}
	if parsed.Hostname() == "" || parsed.Port() == "" {
		return "", errors.New("candidate lacks host or port")
	}
	switch parsed.Scheme {
	case "vless", "trojan", "ss":
	default:
		return "", errors.New("unsupported candidate scheme")
	}
	return net.JoinHostPort(parsed.Hostname(), parsed.Port()), nil
}

func tcpReachable(ctx context.Context, resolver Resolver, endpoint string) error {
	dial := safeDialContext(resolver, &net.Dialer{Timeout: 5 * time.Second})
	connection, err := dial(ctx, "tcp", endpoint)
	if err != nil {
		return err
	}
	return connection.Close()
}

func fingerprint(key []byte, candidate string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(candidate))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func publishAtomically(workDir string, candidates []string) error {
	directory := filepath.Join(workDir, "published")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	contents := []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(candidates, "\n"))))
	temporary, err := os.CreateTemp(directory, ".subscription-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	active := filepath.Join(directory, "subscription.txt")
	backup := filepath.Join(directory, "subscription.previous.txt")
	if _, err := os.Stat(active); err == nil {
		_ = os.Remove(backup)
		if err := os.Rename(active, backup); err != nil {
			return err
		}
	}
	return os.Rename(temporaryName, active)
}
