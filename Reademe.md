# Video Transcoder Proxy

This project is a simple HTTP proxy server written in Go that fetches video streams from an upstream server, transcodes them to 480p using hardware-accelerated Intel Quick Sync Video (QSV) via FFmpeg, and streams the result to clients in MPEG-TS format.

## Features

- Fetches video from an upstream server (default: `http://frigate:5000`)
- Transcodes video to 854x480 resolution using Intel QSV hardware acceleration
- Streams output as MPEG-TS (`video/ts`) to clients
- Passes through audio without re-encoding
- Gracefully handles client disconnects and upstream errors
- Runs in a Docker container with FFmpeg and QSV support

## Usage

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

### Configuration

- **Upstream server:** Change the `UPSTREAM_HOST` constant in [`main.go`](main.go) if your upstream server is not `http://frigate:5000`.
- **FFmpeg options:** Adjust the FFmpeg command in [`main.go`](main.go) to change transcoding parameters.

### Requirements

- Docker (with access to Intel QSV hardware if using hardware acceleration)
- The upstream server must be accessible from the container

## File Structure

- [`main.go`](main.go): Go HTTP proxy and transcoder source code
- [`Dockerfile`](Dockerfile): Multi-stage build for Go and FFmpeg
- [`run.sh`](run.sh): Build and run helper script

## Notes

- TLS verification is disabled for upstream connections.
- The container uses the `linuxserver/ffmpeg` image for FFmpeg with QSV support.
- `/dev/dri` is passed to the container for hardware acceleration.

## License

MIT License (add your license here)