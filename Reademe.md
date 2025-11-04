# Video Transcoder Proxy

This project is a simple HTTP proxy server written in Go that fetches video streams from an upstream server, transcodes them to 480p using hardware-accelerated Intel Quick Sync Video (QSV) via FFmpeg, and streams the result to clients in MPEG-TS format.

## Features

- Fetches video from an upstream server
- Transcodes video to 854x480 resolution using Intel QSV hardware acceleration
- Streams output as MPEG-TS (`video/ts`) to clients
- Passes through audio without re-encoding

## Usage

This container is designed to be used with caddy & frigate with the follow caddy configuration. This will intercept requests for `*.ts` files and transcode them.
```
@ts_files {
   path *.ts
}
reverse_proxy @ts_files http://<transcoder host>:8080
```

## Development

### Build and Run with Docker

1. **Build the Docker image:**
   ```sh
   ./run.sh
   ```

   This script builds the Go binary and the Docker image, then runs the container.

2. **Access the proxy:**
   - The server listens on port `8080`.
   - Example: `http://localhost:8080/<video-path>`

   The proxy will fetch the corresponding path from the upstream server, transcode, and stream it.

### Requirements

- Docker (with access to Intel QSV hardware if using hardware acceleration)
- The upstream server must be accessible from the container

## Notes

- TLS verification is disabled for upstream connections.
- The container uses the `linuxserver/ffmpeg` image for FFmpeg with QSV support.
- `/dev/dri` is passed to the container for hardware acceleration.

## License

MIT License (add your license here)