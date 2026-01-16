# Build the manager binary
FROM golang:1.25-alpine@sha256:e6898559d553d81b245eb8eadafcb3ca38ef320a9e26674df59d4f07a4fd0b07 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG ENABLE_COVERAGE=false

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build with optional coverage instrumentation
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
      CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -cover -covermode=atomic -tags=cover -a -o manager ./cmd/manager; \
    else \
      CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager ./cmd/manager; \
    fi

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:cba10d7abd3e203428e86f5b2d7fd5eb7d8987c387864ae4996cf97191b33764
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
