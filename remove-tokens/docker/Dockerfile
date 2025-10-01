FROM --platform=$BUILDPLATFORM golang:1.24-alpine3.22 AS builder
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
ENV GOSUMDB=off

WORKDIR /app
RUN apk add --no-cache git=2.49.1-r0

# Copy the Go Modules manifests
COPY ../go.mod go.mod
COPY ../go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
# Copy the go source
COPY ../remove-tokens.go remove-tokens.go
RUN go mod tidy && go mod download

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o manager remove-tokens.go

FROM alpine:3.22

WORKDIR /
COPY --from=builder --chown=${USER_UID} /app/manager .
# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk upgrade --no-cache --available

USER 1000:0

ENTRYPOINT ["/manager"]
