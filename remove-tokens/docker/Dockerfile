FROM artifactorycn.netcracker.com:17064/golang:1.24-alpine3.22 as builder

ENV GOPROXY=https://artifactorycn.netcracker.com/pd.sandbox-staging.go.group \
    GOSUMDB=off
COPY docker/config/alpine-repositories /etc/apk/repositories

WORKDIR /app
COPY .. .
COPY ../go.mod go.mod
COPY ../go.sum go.sum
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager remove-tokens.go

FROM artifactorycn.netcracker.com:17064/alpine:3.22

COPY docker/config/alpine-repositories /etc/apk/repositories
COPY --from=builder --chown=${USER_UID} /app/manager .
# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk upgrade --no-cache --available

USER 1000:0

ENTRYPOINT ["/manager"]
