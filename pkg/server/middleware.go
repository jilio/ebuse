package server

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.written += n
	return n, err
}

// loggingMiddleware logs all HTTP requests with structured logging
func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 0}

		// Call next handler
		next(wrapped, r)

		// Log request details
		duration := time.Since(start)

		// Extract IP
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.Split(forwarded, ",")[0]
		}

		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", duration.Milliseconds(),
			"bytes", wrapped.written,
			"ip", ip,
			"user_agent", r.UserAgent(),
		)
	}
}

// gzipResponseWriter wraps http.ResponseWriter to support gzip compression
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// compressionMiddleware adds gzip compression for large responses
func compressionMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r)
			return
		}

		// Create gzip writer
		gz := gzip.NewWriter(w)
		defer gz.Close()

		// Set compression header
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Let gzip set this

		// Wrap response writer
		gzipWriter := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		next(gzipWriter, r)
	}
}

// rateLimiter implements per-IP rate limiting
type rateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
	cleanup  *time.Ticker
}

func newRateLimiter(requestsPerSecond int, burst int) *rateLimiter {
	rl := &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(requestsPerSecond),
		burst:    burst,
		cleanup:  time.NewTicker(5 * time.Minute),
	}

	// Cleanup old limiters periodically
	go func() {
		for range rl.cleanup.C {
			rl.mu.Lock()
			// Simple cleanup: remove all limiters periodically
			// In production, you might want more sophisticated LRU
			rl.limiters = make(map[string]*rate.Limiter)
			rl.mu.Unlock()
		}
	}()

	return rl
}

func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.limiters[ip]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	limiter, exists = rl.limiters[ip]
	if exists {
		return limiter
	}

	limiter = rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[ip] = limiter
	return limiter
}

func (rl *rateLimiter) middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract IP from request
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.Split(forwarded, ",")[0]
		}

		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			slog.Warn("Rate limit exceeded",
				"ip", ip,
				"path", r.URL.Path,
				"method", r.Method)
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}

// Stop stops the rate limiter cleanup
func (rl *rateLimiter) Stop() {
	rl.cleanup.Stop()
}
