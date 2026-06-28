# Production: Hetzner VPS via Dokploy — use Build Type "Dockerfile" or "Docker Compose".
# Do NOT use Nixpacks (default): it runs `go build` with no ffmpeg.
FROM golang:1.24-alpine

# Slideshow / land video generation
RUN apk add --no-cache ffmpeg ttf-dejavu ca-certificates \
	&& ffmpeg -version

ENV FFMPEG_PATH=/usr/bin/ffmpeg
ENV SLIDESHOW_FONT_PATH=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-s -w" -o server .

EXPOSE 4000

CMD ["./server"]
