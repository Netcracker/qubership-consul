# Build the manager binary
FROM golang:1.22.4-alpine3.20 as builder

ENV GOSUMDB=off

WORKDIR /workspace

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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:3.20.2

WORKDIR /
COPY docker/config/alpine-repositories /etc/apk/repositories
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