FROM golang:1.22-bookworm AS builder
WORKDIR /app
COPY main.go .
RUN go build -o transcoder main.go

FROM linuxserver/ffmpeg
WORKDIR /app
COPY --from=builder /app/transcoder .
ENTRYPOINT ["/app/transcoder"]
