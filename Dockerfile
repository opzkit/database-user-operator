# Build the manager binary
FROM golang:1.27-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS builder

ARG TARGETOS
ARG TARGETARCH
ARG ENABLE_COVERAGE=false
ARG VERSION=dev
ARG GIT_COMMIT=none
ARG BUILD_DATE=unknown

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Build with exactly the toolchain go.mod names, as setup-go does for the unit
# tests: make merge-coverage concatenates both coverage profiles, and go tool
# cover rejects blocks counted by two Go versions. The base image's Go patch
# release moves on its own schedule, and GOTOOLCHAIN=auto would only ever
# switch to a newer one, so the name is read from go.mod.
#
# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN GOTOOLCHAIN="$(awk '$1 == "toolchain" {print $2}' go.mod)" && export GOTOOLCHAIN="${GOTOOLCHAIN:-local}" && \
    go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build with optional coverage instrumentation. Build metadata stamped via -ldflags.
RUN GOTOOLCHAIN="$(awk '$1 == "toolchain" {print $2}' go.mod)" && export GOTOOLCHAIN="${GOTOOLCHAIN:-local}" && \
    LDFLAGS="-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildDate=${BUILD_DATE}" && \
    if [ "$ENABLE_COVERAGE" = "true" ]; then \
      CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -cover -covermode=atomic -tags=cover -a -ldflags "$LDFLAGS" -o manager ./cmd/manager; \
    else \
      CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -ldflags "$LDFLAGS" -o manager ./cmd/manager; \
    fi

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
