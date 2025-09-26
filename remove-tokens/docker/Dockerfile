FROM --platform=$BUILDPLATFORM golang:1.24-alpine3.22 as builder
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
ENV GOSUMDB=off
COPY docker/config/alpine-repositories /etc/apk/repositories

WORKDIR /app
COPY .. .
COPY ../go.mod go.mod
COPY ../go.sum go.sum
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o manager remove-tokens.go

FROM alpine:3.22

COPY --from=builder --chown=${USER_UID} /app/manager .
# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk upgrade --no-cache --available

USER 1000:0

ENTRYPOINT ["/manager"]
