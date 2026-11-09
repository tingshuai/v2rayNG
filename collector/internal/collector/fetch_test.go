package collector

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

type staticResolver struct{ addresses []netip.Addr }

func (resolver staticResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return resolver.addresses, nil
}

func TestValidatePublicHostRejectsNonPublicResolution(t *testing.T) {
	resolver := staticResolver{addresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	if err := validatePublicHost(context.Background(), resolver, "source.example"); err == nil {
		t.Fatal("private resolution was accepted")
	}
}

func TestSplitCandidatesRejectsOversizedCandidateSet(t *testing.T) {
	body := make([]byte, 0, maxSourceCandidates*10)
	for range maxSourceCandidates + 1 {
		body = append(body, "candidate "...)
	}
	if _, err := splitCandidates(body); err == nil {
		t.Fatal("oversized candidate set was accepted")
	}
}

func TestResolveSourceURLExpandsDailyTokenInUTC(t *testing.T) {
	now := time.Date(2026, time.November, 9, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	got := resolveSourceURL("https://source.example/{YYYYMMDD}-v2ray.txt", now)
	if got != "https://source.example/20261108-v2ray.txt" {
		t.Fatalf("resolved URL = %q", got)
	}
}
