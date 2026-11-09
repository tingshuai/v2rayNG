package collector

import (
	"context"
	"encoding/base64"
	"net/netip"
	"net/url"
	"strconv"
	"testing"
)

func TestSingBoxConfigUsesResolvedPublicAddressAndTLSName(t *testing.T) {
	candidate := (&url.URL{
		Scheme:   "vless",
		User:     url.User("test-credential"),
		Host:     "node.example:443",
		RawQuery: url.Values{"security": []string{"tls"}, "sni": []string{"edge.example"}}.Encode(),
	}).String()
	prober := SingBoxProber{Resolver: staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}}
	config, err := prober.config(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	outbound := config.Outbounds[0]
	if outbound["server"] != "8.8.8.8" || outbound["server_port"] != 443 {
		t.Fatalf("unexpected resolved endpoint: %#v", outbound)
	}
	tls, ok := outbound["tls"].(map[string]any)
	if !ok || tls["server_name"] != "edge.example" {
		t.Fatalf("unexpected TLS settings: %#v", outbound["tls"])
	}
}

func TestSingBoxConfigRejectsUnsupportedTransport(t *testing.T) {
	candidate := (&url.URL{
		Scheme:   "trojan",
		User:     url.User("test-credential"),
		Host:     "node.example:443",
		RawQuery: url.Values{"type": []string{"ws"}}.Encode(),
	}).String()
	prober := SingBoxProber{Resolver: staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}}
	if _, err := prober.config(context.Background(), candidate); err == nil {
		t.Fatal("unsupported transport was accepted")
	}
}

func TestSingBoxConfigSupportsEncodedShadowsocksCredentials(t *testing.T) {
	credentials := base64.RawStdEncoding.EncodeToString([]byte("aes-256-gcm:test-password"))
	candidate := (&url.URL{Scheme: "ss", User: url.User(credentials), Host: "node.example:443"}).String()
	prober := SingBoxProber{Resolver: staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}}
	config, err := prober.config(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	outbound := config.Outbounds[0]
	if outbound["type"] != "shadowsocks" || outbound["method"] != "aes-256-gcm" || outbound["password"] != "test-password" {
		t.Fatalf("unexpected shadowsocks outbound: %#v", outbound)
	}
}

func TestSelectProbeCandidatesSkipsUnsupportedEntriesBeforeBudgeting(t *testing.T) {
	unsupported := make([]string, maxScreenedCandidates)
	for index := range unsupported {
		unsupported[index] = "vmess://unsupported"
	}
	candidates := append(unsupported, "trojan://test-credential@node.example:443")
	selected := selectProbeCandidates(candidates)
	if len(selected) != 1 || selected[0] != candidates[len(candidates)-1] {
		t.Fatalf("selected candidates = %#v", selected)
	}
}

func TestSelectProbeCandidatesSamplesAcrossEligibleSet(t *testing.T) {
	candidates := make([]string, maxScreenedCandidates*2)
	for index := range candidates {
		candidates[index] = "trojan://test-credential@node" + strconv.Itoa(index) + ".example:443"
	}
	selected := selectProbeCandidates(candidates)
	if len(selected) != maxScreenedCandidates {
		t.Fatalf("selected count = %d, want %d", len(selected), maxScreenedCandidates)
	}
	if selected[len(selected)-1] != candidates[len(candidates)-2] {
		t.Fatalf("last sampled candidate = %q, want %q", selected[len(selected)-1], candidates[len(candidates)-2])
	}
}

func TestValidateProbeURLRejectsPrivateTarget(t *testing.T) {
	resolver := staticResolver{addresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	if err := validateProbeURL(context.Background(), resolver, "https://probe.example/generate_204"); err == nil {
		t.Fatal("private probe URL target was accepted")
	}
}
