package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const defaultProbeURL = "https://cp.cloudflare.com/generate_204"

type CandidateProber interface {
	Probe(ctx context.Context, candidate string) error
}

type SingBoxProber struct {
	BinaryPath string
	ProbeURL   string
	Resolver   Resolver
	WorkDir    string
}

func (prober SingBoxProber) Probe(ctx context.Context, candidate string) error {
	if prober.BinaryPath == "" {
		return errors.New("sing-box binary path is required")
	}
	if prober.Resolver == nil {
		prober.Resolver = net.DefaultResolver
	}
	probeURL := prober.ProbeURL
	if probeURL == "" {
		probeURL = defaultProbeURL
	}
	if err := validateProbeURL(ctx, prober.Resolver, probeURL); err != nil {
		return err
	}
	config, err := prober.config(ctx, candidate)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return err
	}
	config.Inbounds[0].ListenPort = port
	contents, err := json.Marshal(config)
	if err != nil {
		return err
	}
	directory := filepath.Join(prober.WorkDir, "probe")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, "sing-box-*.json")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(contents); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, prober.BinaryPath, "run", "-c", path)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return fmt.Errorf("start sing-box probe: %w", err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	proxyURL, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, Timeout: 5 * time.Second}
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusNoContent {
					return nil
				}
				return fmt.Errorf("probe endpoint returned status %d", response.StatusCode)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("sing-box probe did not become ready")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func validateProbeURL(ctx context.Context, resolver Resolver, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("probe URL must be an unauthenticated HTTPS URL without a fragment")
	}
	return validatePublicHost(ctx, resolver, parsed.Hostname())
}

type singBoxConfig struct {
	Log       map[string]any    `json:"log"`
	Inbounds  []singBoxInbound  `json:"inbounds"`
	Outbounds []map[string]any  `json:"outbounds"`
	Route     map[string]string `json:"route"`
}

type singBoxInbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Listen     string `json:"listen"`
	ListenPort int    `json:"listen_port"`
}

func (prober SingBoxProber) config(ctx context.Context, candidate string) (singBoxConfig, error) {
	parsed, err := url.Parse(candidate)
	if err != nil {
		return singBoxConfig{}, err
	}
	if parsed.Port() == "" || parsed.User == nil {
		return singBoxConfig{}, errors.New("candidate lacks required connection fields")
	}
	endpoint, err := publicEndpoint(ctx, prober.Resolver, parsed.Hostname(), parsed.Port())
	if err != nil {
		return singBoxConfig{}, err
	}
	transport := parsed.Query().Get("type")
	if transport != "" && transport != "tcp" {
		return singBoxConfig{}, errors.New("candidate transport is not supported by bounded probe")
	}
	outbound := map[string]any{"tag": "probe-out", "server": endpoint, "server_port": mustPort(parsed.Port())}
	switch parsed.Scheme {
	case "vless":
		security := parsed.Query().Get("security")
		if security != "" && security != "none" && security != "tls" {
			return singBoxConfig{}, errors.New("candidate security mode is not supported by bounded probe")
		}
		outbound["type"] = "vless"
		outbound["uuid"] = parsed.User.Username()
		if security == "tls" {
			outbound["tls"] = map[string]any{"enabled": true, "server_name": serverName(parsed)}
		}
	case "trojan":
		outbound["type"] = "trojan"
		outbound["password"] = parsed.User.Username()
		outbound["tls"] = map[string]any{"enabled": true, "server_name": serverName(parsed)}
	default:
		return singBoxConfig{}, errors.New("candidate protocol is not supported by bounded probe")
	}
	return singBoxConfig{Log: map[string]any{"disabled": true}, Inbounds: []singBoxInbound{{Type: "mixed", Tag: "probe-in", Listen: "127.0.0.1"}}, Outbounds: []map[string]any{outbound}, Route: map[string]string{"final": "probe-out"}}, nil
}

func publicEndpoint(ctx context.Context, resolver Resolver, host, port string) (string, error) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return "", err
	}
	for _, address := range addresses {
		if IsPublicAddress(address) {
			return address.String(), nil
		}
	}
	return "", errors.New("candidate host has no public address")
}

func serverName(parsed *url.URL) string {
	if value := parsed.Query().Get("sni"); value != "" {
		return value
	}
	return parsed.Hostname()
}

func mustPort(value string) int { port, _ := strconv.Atoi(value); return port }
