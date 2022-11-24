// Package gzip provides gzip middleware to gzip responses where appropriate
package gzip

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// This middleware provides gzip compression on requests where the client accepts it
// This code is a simplified version of the gzip middleware provided with Gorilla by various authors
// For example it does not offer level config or support deflate options
// See https://github.com/gorilla for original copyright

// Middleware gzips responses where the request includes the Accept-Encoding header
func Middleware(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		// If gzip not accepted, execute the handler without compression and return
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h(w, r)
			return
		}

		// Get ready to gzip the response by adding headers
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")

		// Create a new writer to write the content, close after this function returns
		gw, _ := gzip.NewWriterLevel(w, gzip.DefaultCompression)
		defer gw.Close()

		// Store Hijacker, Flusher, Notifier if appropriate
		hijacker, hok := w.(http.Hijacker)
		if !hok {
			hijacker = nil
		}

		flusher, fok := w.(http.Flusher)
		if !fok {
			flusher = nil
		}

		notifier, cnok := w.(http.CloseNotifier)
		if !cnok {
			notifier = nil
		}

		// Replace the writer with the compressed response writer
		w = &compressResponseWriter{
			Writer:         gw,
			ResponseWriter: w,
			Hijacker:       hijacker,
			Flusher:        flusher,
			CloseNotifier:  notifier,
		}

		// Call the handler with the new writer
		h(w, r)

	}
}

type compressResponseWriter struct {
	io.Writer
	http.ResponseWriter
	http.Hijacker
	http.Flusher
	http.CloseNotifier
}

// WriteHeader writes the header and zeroes content length if set
// Content-Type should be set on all zero length responses.
func (w *compressResponseWriter) WriteHeader(c int) {
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(c)
}

// Header returns the underlying writer header
func (w *compressResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// Write ensures content length is 0 and writes to the gzip writer.
// If Content-Type is not set it attempts to sniff it using http.DetectContentType.
func (w *compressResponseWriter) Write(b []byte) (int, error) {
	h := w.ResponseWriter.Header()
	if h.Get("Content-Type") == "" {
		h.Set("Content-Type", http.DetectContentType(b))
	}
	h.Del("Content-Length")

	return w.Writer.Write(b)
}

type flusher interface {
	Flush() error
}

func (w *compressResponseWriter) Flush() {
	// Flush compressed data if compressor supports it.
	if f, ok := w.Writer.(flusher); ok {
		f.Flush()
	}
	// Flush HTTP response.
	if w.Flusher != nil {
		w.Flusher.Flush()
	}
}
