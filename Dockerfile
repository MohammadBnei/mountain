# atlas image: one static Go binary with www/ embedded (embed.go). Built by .github/workflows/app.yml.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY embed.go ./
COPY internal ./internal
COPY cmd ./cmd
COPY www ./www
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /atlas ./cmd/atlas

# distroless static:nonroot runs as 65532; it writes only to the cache volume (/var/cache/atlas, helm/values.yaml).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /atlas /atlas
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/atlas"]
