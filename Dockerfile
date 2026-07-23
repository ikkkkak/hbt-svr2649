# syntax=docker/dockerfile:1
# Dokploy Application deploy:
#   Build type = Dockerfile
#   Dockerfile path = Dockerfile   (never docker-compose.yaml or compose.yaml)
#   Build path = .  (or apartmentscloneserver if repo root is parent)
FROM golang:1.25-alpine

RUN apk add --no-cache ffmpeg ttf-dejavu ca-certificates \
	&& ffmpeg -version

ENV FFMPEG_PATH=/usr/bin/ffmpeg
ENV SLIDESHOW_FONT_PATH=/usr/share/fonts/dejavu/DejaVuSans-Bold.ttf
# Chromium REMOVED — it exhausted the VM's RAM/disk and slowed builds.
# JS-rendered sites are handled via the admin "Paste external JSON" tool.

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
	go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	go build -p 2 -ldflags="-s -w" -o server .

EXPOSE 4000

CMD ["./server"]
