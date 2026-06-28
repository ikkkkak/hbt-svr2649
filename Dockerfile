# Dokploy: Build Type MUST be "Dockerfile" — Dockerfile path = Dockerfile (NOT docker-compose.yaml).
# Compose file is only for "Docker Compose" deploy type (docker compose up --build).
FROM golang:1.24-alpine
# Slideshow / land video generation
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
