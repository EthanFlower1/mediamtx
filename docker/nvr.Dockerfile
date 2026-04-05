###############################################################
# MediaMTX NVR - Multi-stage Docker build
# Supports linux/amd64 and linux/arm64
###############################################################

# ---------- Stage 1: Build React admin UI ----------
FROM node:20-alpine3.22 AS build-ui

WORKDIR /ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npx vite build --outDir /ui-dist

# ---------- Stage 2: Build Go binary ----------
FROM golang:1.25-alpine3.22 AS build-go

RUN apk add --no-cache git make

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=build-ui /ui-dist/ /src/internal/nvr/ui/dist/

ENV CGO_ENABLED=0
RUN go generate ./...
RUN go build -o /mediamtx .

# ---------- Stage 3: Minimal runtime ----------
FROM alpine:3.22

RUN apk add --no-cache \
    ffmpeg \
    ca-certificates \
    tzdata

# Create non-root user (but allow running as root via compose override)
RUN addgroup -S mediamtx && adduser -S mediamtx -G mediamtx

# Data directories
RUN mkdir -p /config /data /recordings && \
    chown -R mediamtx:mediamtx /config /data /recordings

COPY --from=build-go /mediamtx /mediamtx
COPY --from=build-go /src/mediamtx.yml /config/mediamtx.yml

# Default environment variables — override at runtime
ENV MTX_LOGLEVEL=info
ENV MTX_API=yes
ENV MTX_NVR=yes
ENV MTX_PLAYBACK=yes

# Ports:
#  8554  - RTSP (TCP)
#  8000  - RTSP (UDP RTP)
#  8001  - RTSP (UDP RTCP)
#  1935  - RTMP
#  8888  - HLS
#  8889  - WebRTC (HTTP)
#  8189  - WebRTC (ICE/UDP)
#  9996  - API / Playback
#  9997  - Metrics (Prometheus)
EXPOSE 8554 8000/udp 8001/udp 1935 8888 8889 8189/udp 9996 9997

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:9997/v3/paths/list || exit 1

VOLUME ["/config", "/data", "/recordings"]

ENTRYPOINT ["/mediamtx"]
CMD ["/config/mediamtx.yml"]
