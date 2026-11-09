package collector

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	maxSourceBytes      = 2 << 20
	maxSourceCandidates = 5000
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type SafeFetcher struct {
	Resolver Resolver
	Client   *http.Client
}

func NewSafeFetcher(resolver Resolver) *SafeFetcher {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, DialContext: safeDialContext(resolver, dialer), TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second}
	return &SafeFetcher{Resolver: resolver, Client: &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (fetcher *SafeFetcher) Fetch(ctx context.Context, source Source) ([]byte, error) {
	parsed, err := url.Parse(source.FetchURL)
	if err != nil {
		return nil, err
	}
	if err := validatePublicHost(ctx, fetcher.Resolver, parsed.Hostname()); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.FetchURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := fetcher.Client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch source: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected source status: %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, maxSourceBytes+1))
}

func safeDialContext(resolver Resolver, dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, candidate := range addresses {
			if IsPublicAddress(candidate) {
				return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			}
		}
		return nil, errors.New("resolved target has no public address")
	}
}

func validatePublicHost(ctx context.Context, resolver Resolver, host string) error {
	if host == "" {
		return errors.New("missing source host")
	}
	if literal, err := netip.ParseAddr(host); err == nil {
		if !IsPublicAddress(literal) {
			return errors.New("source host is not public")
		}
		return nil
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolve source host: %w", err)
	}
	for _, candidate := range addresses {
		if IsPublicAddress(candidate) {
			return nil
		}
	}
	return errors.New("source host has no public address")
}

func splitCandidates(body []byte) ([]string, error) {
	if len(body) > maxSourceBytes {
		return nil, errors.New("source body exceeds limit")
	}
	lines := strings.Fields(string(body))
	if len(lines) == 1 && !strings.Contains(lines[0], "://") {
		decoded, err := base64.StdEncoding.DecodeString(lines[0])
		if err == nil {
			lines = strings.Fields(string(decoded))
		}
	}
	if len(lines) == 0 {
		return nil, errors.New("source contained no candidates")
	}
	if len(lines) > maxSourceCandidates {
		return nil, errors.New("source candidate count exceeds limit")
	}
	return lines, nil
}
