FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /collector ./cmd/collector

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /collector /app/collector
COPY --from=build /src/migrations /app/migrations
USER nonroot:nonroot
ENTRYPOINT ["/app/collector"]
CMD ["serve", "-database", "/data/collector.sqlite", "-migrations", "/app/migrations", "-work-dir", "/data", "-listen", "127.0.0.1:8080"]
