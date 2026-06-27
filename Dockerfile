# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.3

# ---- Frontend build stage ----
FROM --platform=$BUILDPLATFORM node:22-alpine AS front-builder
WORKDIR /app/temp_frontend
# sync-version.mjs reads ../config/version, so bring scripts + config/version too.
COPY temp_frontend/package.json temp_frontend/package-lock.json ./
COPY scripts/ /app/scripts/
COPY config/version /app/config/version
RUN npm ci
COPY temp_frontend/ ./
RUN npm run build

# ---- Backend build stage (pure Go, no CGO) ----
FROM golang:${GO_VERSION}-alpine AS backend-builder
WORKDIR /app
ARG TARGETARCH
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=$TARGETARCH
RUN apk add --no-cache git
COPY . .
# Replace embedded frontend assets with the freshly built ones
RUN rm -rf web/html && mkdir -p web/html
COPY --from=front-builder /app/temp_frontend/dist/ /app/web/html/
RUN go build -ldflags="-w -s" -o kwor main.go

# ---- Runtime stage ----
FROM alpine
LABEL org.opencontainers.image.source="https://github.com/nicelic/kwor"
ENV KWOR_RUNTIME_MODE=docker
WORKDIR /app
RUN set -ex && apk add --no-cache --upgrade bash tzdata ca-certificates nftables curl wget iproute2 openssl procps tar unzip socat
COPY --from=backend-builder /app/kwor /app/
COPY entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh /app/kwor
ENTRYPOINT ["/app/entrypoint.sh"]
