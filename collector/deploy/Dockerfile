FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /collector ./cmd/collector

FROM ghcr.io/sagernet/sing-box:v1.14.4@sha256:0788ffbddab2c68a99a2dd4780bbc50902f159af6f589df3ee74a8389d278279 AS sing-box

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /collector /app/collector
COPY --from=build /src/migrations /app/migrations
COPY --from=sing-box /usr/local/bin/sing-box /usr/local/bin/sing-box
USER nonroot:nonroot
ENTRYPOINT ["/app/collector"]
CMD ["serve", "-database", "/data/collector.sqlite", "-migrations", "/app/migrations", "-work-dir", "/data", "-listen", "127.0.0.1:8080"]
