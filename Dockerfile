# Relay image: one static Go binary with www/ embedded (embed.go). Built by .github/workflows/app.yml.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY embed.go units.json ./
COPY relay ./relay
COPY www ./www
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /relay ./relay

# distroless static:nonroot runs as 65532; nothing writes to disk.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /relay /relay
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/relay"]
