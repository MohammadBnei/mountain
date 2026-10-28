# atlas image: one static Go binary with www/ embedded (embed.go). Built by .github/workflows/app.yml.
FROM golang:1.27 AS build
WORKDIR /src
# mountain's nested module (pkg/) is private: go mod download reads it with the build job's token, a secret mount
# applied through git's env config for that one command, so it is in no layer and no image.
ENV GOPRIVATE=github.com/MohammadBnei/mountain
COPY go.mod go.sum ./
RUN --mount=type=secret,id=github_token \
    [ -s /run/secrets/github_token ] || { echo "the github_token build secret is missing or empty" >&2; exit 1; }; \
    GIT_CONFIG_COUNT=1 \
      GIT_CONFIG_KEY_0="url.https://x-access-token:$(cat /run/secrets/github_token)@github.com/MohammadBnei/mountain.insteadOf" \
      GIT_CONFIG_VALUE_0="https://github.com/MohammadBnei/mountain" \
      go mod download
COPY embed.go ./
COPY internal ./internal
COPY contract ./contract
COPY realms ./realms
COPY cmd ./cmd
COPY www ./www
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /atlas ./cmd/atlas

# distroless static:nonroot runs as 65532; it writes only to the cache volume (/var/cache/atlas, helm/values.yaml).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /atlas /atlas
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/atlas"]
