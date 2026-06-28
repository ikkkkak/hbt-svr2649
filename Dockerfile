# Dokploy Application deploy:
#   Build type = Dockerfile
#   Dockerfile path = Dockerfile   (never docker-compose.yaml or compose.yaml)
#   Build path = .  (or apartmentscloneserver if repo root is parent)
FROM golang:1.24-alpine

RUN apk add --no-cache ffmpeg ttf-dejavu ca-certificates \
	&& ffmpeg -version

ENV FFMPEG_PATH=/usr/bin/ffmpeg
ENV SLIDESHOW_FONT_PATH=/usr/share/fonts/dejavu/DejaVuSans-Bold.ttf

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-s -w" -o server .

EXPOSE 4000

CMD ["./server"]
