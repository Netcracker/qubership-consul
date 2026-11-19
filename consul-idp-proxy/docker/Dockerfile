FROM --platform=$BUILDPLATFORM golang:1.27-alpine3.24 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY internal/ internal/
RUN CGO_ENABLED=0 go test ./...
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o consul-idp-proxy .

FROM alpine:3.24
RUN apk add --no-cache ca-certificates
COPY --from=builder /workspace/consul-idp-proxy /consul-idp-proxy
USER 1000:0
EXPOSE 8080
ENTRYPOINT ["/consul-idp-proxy"]
