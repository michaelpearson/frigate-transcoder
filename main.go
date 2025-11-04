package main

import (
	"crypto/tls"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"time"
)

const UPSTREAM_HOST = "http://frigate:5000"
const LISTEN_ADDR = ":8080"

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

		// Decode
    "-c:v", "h264_qsv", 

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
	defer stdinPipe.Close() // Ensure stdinPipe is closed after we finish with it

	// Use an in-memory buffer to capture stderr, which is easier to manage
	// than an asynchronous pipe copy in a robust program.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	
	// Create stdout pipe
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Error creating stdout pipe: %v", err)
		http.Error(w, "Internal server error.", http.StatusInternalServerError)
		return
	}

	// 2. Fetch Upstream Data
	upstreamURL := UPSTREAM_HOST + r.URL.Path
	log.Printf("Transcoding request received for: %s. Fetching upstream: %s", r.URL.Path, upstreamURL)
	
	// Use the request context for the upstream fetch as well, so we don't
	// fetch a huge file if the client has already cancelled.
	req, _ := http.NewRequestWithContext(ctx, "GET", upstreamURL, nil)
	upstreamResp, err := insecureClient.Do(req)
	
	if err != nil {
		log.Printf("Error fetching upstream URL %s: %v", upstreamURL, err)
		// Check for context cancellation error specifically
		if ctx.Err() == context.Canceled {
			return // Client cancelled, just exit gracefully
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

	// 3. Start FFmpeg and Pipe Data
	if err := cmd.Start(); err != nil {
		log.Printf("Error starting FFmpeg: %v. Make sure FFmpeg is installed and in PATH.", err)
		http.Error(w, "Transcoding service unavailable (FFmpeg failed to start).", http.StatusInternalServerError)
		return
	}
	log.Printf("FFmpeg process started (PID: %d)", cmd.Process.Pid)

	// Goroutine to stream upstream body into FFmpeg's stdin
	go func() {
		defer stdinPipe.Close() // Closes stdin, signalling EOF to FFmpeg
		
		written, err := io.Copy(stdinPipe, upstreamResp.Body)
		if err != nil && err != io.EOF && ctx.Err() == nil {
			// Log error only if the request wasn't already canceled
			log.Printf("Error copying upstream body to FFmpeg stdin: %v. Bytes written: %d", err, written)
		}
	}()

	// 4. Stream FFmpeg Output to Client
	w.Header().Set("Content-Type", "video/ts")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")

	written, err := io.Copy(w, stdoutPipe)
	if err != nil {
		// This error is likely a client disconnect (EPIPE or similar)
		log.Printf("Error copying FFmpeg output to client: %v. Written bytes: %d", err, written)
		// Since FFmpeg is running with CommandContext, it will receive the
		// cancellation signal and terminate automatically.
	}

	// 5. Wait for FFmpeg and Log Status
	waitErr := cmd.Wait()
	
	// Log FFmpeg's stderr output after the process exits
	if stderrBuf.Len() > 0 {
		log.Printf("FFmpeg Stderr (PID: %d):\n%s", cmd.Process.Pid, stderrBuf.String())
	}

	if waitErr != nil {
		if ctx.Err() == context.Canceled {
			// This is expected if the client disconnects and CommandContext kills FFmpeg
			log.Printf("FFmpeg process (PID: %d) terminated by client cancellation.", cmd.Process.Pid)
		} else {
			// Log other errors, e.g., FFmpeg error status code
			log.Printf("FFmpeg exited with error (PID: %d): %v", cmd.Process.Pid, waitErr)
		}
	} else {
		log.Printf("Transcoding and streaming complete for %s. Total bytes sent: %d", r.URL.Path, written)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", transcodeStream)

	srv := &http.Server{
		Addr:         LISTEN_ADDR,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Transcoding proxy server starting on %s, forwarding to %s", LISTEN_ADDR, UPSTREAM_HOST)
	log.Printf("NOTE: TLS verification is skipped for upstream connections.")

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on %s: %v", LISTEN_ADDR, err)
	}
}
