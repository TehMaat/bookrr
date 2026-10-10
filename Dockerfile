
# Frontend and Go binary are built on the build machine's native platform and
# cross-compiled for the target, so multi-arch builds need no emulation for
# the heavy steps and work for 386 / armv6 / armv7 / arm64 / amd64.
# Base images come from Google's Docker Hub mirror to avoid Hub rate limits.

FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.24-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
# Empty VERSION (local builds) falls back to the VERSION file.
ARG VERSION=
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY VERSION ./
COPY web/embed.go ./web/
COPY --from=web /web/dist ./web/dist
# timetzdata embeds the time zone database, so TZ works without tzdata.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -tags timetzdata -ldflags="-s -w -X main.version=${VERSION:-v$(cat VERSION)}" -o /out/bookrr ./cmd/bookrr \
 && mkdir -p /out/data

# No RUN steps here: the runtime image needs no emulation to build for any arch.
FROM mirror.gcr.io/library/alpine:3.22
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/bookrr /usr/local/bin/bookrr
COPY --from=build /out/data /data
ENV BOOKRR_DATA_DIR=/data \
    BOOKRR_LISTEN=:8080
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["bookrr", "-healthcheck"]
ENTRYPOINT ["bookrr"]
