# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.24.5-alpine3.21 as builder
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

ARG GH_ACCESS_TOKEN

ENV GOSUMDB=off

WORKDIR /workspace

RUN apk add --no-cache git

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer

RUN go mod download

# Copy the go source
COPY main.go main.go
COPY clients/ clients/

RUN go mod tidy

# Tests
RUN CGO_ENABLED=0 go test -v ./...
# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:3.21.3

WORKDIR /
COPY --from=builder --chown=${USER_UID} /workspace/manager .

# Install misc tools
RUN set -x \
    && apk add --update curl

# Avoiding vulnerabilities
RUN set -x \
    && apk add --upgrade --no-cache apk-tools

# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk upgrade --no-cache --available

USER 1000:0

ENTRYPOINT ["/manager"]
