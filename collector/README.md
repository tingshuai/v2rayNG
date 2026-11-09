# FreeV2ray collector

`collector/` is the independently built and deployed service that turns explicitly registered public sources into a standard V2Ray-compatible candidate subscription. It has no runtime dependency on `V2rayNG/`; the only shared boundary is the versioned contract in [`../docs/free-node/feed-schema.md`](../docs/free-node/feed-schema.md).

The service is intentionally conservative: it rejects authenticated sources and non-public resolved addresses, never scans ports, and retains only secret-safe fingerprints in SQLite. Full candidate configurations exist only in the encrypted runtime work directory while publishing a batch, never in the database or logs.

## Local commands

```sh
go test ./...
go run ./cmd/collector migrate -database ./data/collector.sqlite
go run ./cmd/collector source add -database ./data/collector.sqlite -id example -url https://source.example/subscription
go run ./cmd/collector source update -database ./data/collector.sqlite -id example -url https://source.example/subscription-v2 -min-fetch-interval 30m -reason operator_request
go run ./cmd/collector source pause -database ./data/collector.sqlite -id example -reason operator_request
go run ./cmd/collector source delete -database ./data/collector.sqlite -id example -reason operator_request
go run ./cmd/collector run -database ./data/collector.sqlite -work-dir ./data -fingerprint-key-file /run/secrets/collector_fingerprint_key -sing-box-path /usr/local/bin/sing-box
go run ./cmd/collector serve -database ./data/collector.sqlite -work-dir ./data -listen 127.0.0.1:8080
```

`deploy/compose.yaml` runs only the read-only subscription server. The bounded batch runner is deliberately separate: install the `deploy/systemd/collector-run.{service,timer}` units and set `COLLECTOR_FINGERPRINT_KEY_FILE` in `/etc/freev2ray/collector.env` from the supplied example. The timer invokes `compose.runner.yaml`, which mounts the key as a Docker secret only for that one-shot container. Install the paired health-check timer; `/healthz` returns success only while the current subscription file is newer than its configured maximum age, and its systemd failure hook invokes the optional local alert command.

Commands never print a source URL, candidate configuration, credential, UUID, private key, user IP, or payload. Registration accepts only public, unauthenticated HTTPS URLs; a registered source is not thereby trustworthy or useful on every client network.

Use the deployment sample in `deploy/` for a non-root container and bind the service only to loopback before exposing it through the configured tunnel. Runtime state and deployment secrets are Git-ignored.

Each batch enforces a source-body and candidate-count limit, checks public DNS resolution before the request and again immediately before each TCP connection, and preserves the previous subscription if no screened candidate can be published. Source fetching is capped at four concurrent operations and probing at two globally shared operations. The screening deadline applies only to fetching and probing: candidates already accepted when it expires are recorded and published with a separate short finalization budget. A source that has no usable candidate or is deferred by that batch deadline remains enabled; only repeated fetch or parse failures can trigger automatic pausing. HMAC candidate fingerprints and non-secret publication snapshots are retained in SQLite for the documented 30-day audit window. VLESS and Trojan candidates using direct TCP are then tested by a short-lived, resource-constrained sing-box process through a small HTTPS HTTP 204 request. The process receives an ephemeral `0600` configuration file which is removed when the probe ends; stale probe configurations from interrupted batches are removed before a new batch starts; candidates using an unsupported transport or protocol are rejected rather than guessed.
