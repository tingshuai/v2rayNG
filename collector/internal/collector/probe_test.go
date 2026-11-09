package collector

import (
	"context"
	"net/netip"
	"net/url"
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

func TestValidateProbeURLRejectsPrivateTarget(t *testing.T) {
	resolver := staticResolver{addresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	if err := validateProbeURL(context.Background(), resolver, "https://probe.example/generate_204"); err == nil {
		t.Fatal("private probe URL target was accepted")
	}
}
