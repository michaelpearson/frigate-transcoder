package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

var upstreamHost = os.Getenv("REMOTE_HOST")

var insecureClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
	Timeout: 30 * time.Second,
}

func transcodeStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-hide_banner",
		"-hwaccel", "qsv",
		"-hwaccel_output_format", "qsv",

		// Input
		"-i", "pipe:0",

		// Scale (854x480)
		"-vf", "scale_qsv=w=854:h=480",

		// Encode
		"-c:v", "h264_qsv",

		// Constant quality
		"-rc_mode", "CQP",
		"-q:v", "25",

		// Copy audio
		"-c:a", "copy",

		// Preserve timestamps
		"-copyts",

		// Output TS to stdout
		"-f", "mpegts",
		"pipe:1",
	)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("Error creating stdin pipe: %v", err)
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}

	defer stdinPipe.Close()

	// Capture stderr for logging
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	// Create stdout pipe
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Error creating stdout pipe: %v", err)
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}

	upstreamURL := upstreamHost + r.URL.Path

	req, _ := http.NewRequestWithContext(ctx, "GET", upstreamURL, nil)
	upstreamResp, err := insecureClient.Do(req)

	if err != nil {
		if ctx.Err() == context.Canceled {
			return
		}
		http.Error(w, "Error connecting to upstream source.", http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()

	if upstreamResp.StatusCode != http.StatusOK {
		log.Printf("Upstream returned non-OK status: %d", upstreamResp.StatusCode)
		http.Error(w, fmt.Sprintf("Upstream file not found or error: %s", upstreamResp.Status), upstreamResp.StatusCode)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error starting FFmpeg: %v. Make sure FFmpeg is installed and in PATH", err)
		http.Error(w, "FFmpeg failed to start", http.StatusInternalServerError)
		return
	}

	go func() {
		defer stdinPipe.Close()
		written, err := io.Copy(stdinPipe, upstreamResp.Body)
		if err != nil && err != io.EOF && ctx.Err() == nil {
			log.Printf("Error copying upstream body to FFmpeg stdin: %v. Bytes written: %d", err, written)
		}
	}()

	w.Header().Set("Content-Type", "video/ts")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")

	written, err := io.Copy(w, stdoutPipe)
	if err != nil {
		log.Printf("Error copying FFmpeg output to client: %v. Written bytes: %d", err, written)
	}
	waitErr := cmd.Wait()

	if stderrBuf.Len() > 0 {
		log.Printf("FFmpeg Stderr (PID: %d):\n%s", cmd.Process.Pid, stderrBuf.String())
	}

	if waitErr != nil {
		if ctx.Err() == context.Canceled {
			log.Printf("FFmpeg process (PID: %d) terminated by client cancellation.", cmd.Process.Pid)
		} else {
			log.Printf("FFmpeg exited with error (PID: %d): %v", cmd.Process.Pid, waitErr)
		}
	} else {
		log.Printf("Transcoding and streaming complete for %s. Total bytes sent: %d", r.URL.Path, written)
	}
}

func main() {
	if upstreamHost == "" {
		log.Panic("REMOTE_HOST environment variable is not set. Please set it to the upstream host URL.")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", transcodeStream)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Listening on 8080, forwarding to %s", upstreamHost)
	log.Printf("NOTE: TLS verification is skipped for upstream connections.")

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on 8080: %v", err)
	}
}
