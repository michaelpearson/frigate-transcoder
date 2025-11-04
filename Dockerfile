FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o transcoder ./cmd

FROM linuxserver/ffmpeg
WORKDIR /app
COPY --from=builder /app/transcoder .
ENTRYPOINT ["/app/transcoder"]
