package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// gzipMinBytes is the smallest first write worth compressing; tiny JSON
// bodies grow under gzip framing.
const gzipMinBytes = 1024

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
		return w
	},
}

// compressibleType reports whether a response Content-Type benefits from gzip.
// Binary payloads (receipt images, PDFs) are already compressed or not worth
// it, and are left alone.
func compressibleType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(ct, "application/json") ||
		strings.HasPrefix(ct, "text/") ||
		strings.HasPrefix(ct, "application/javascript") ||
		strings.HasPrefix(ct, "image/svg+xml")
}

// gzipResponseWriter decides on its first write whether to compress, based on
// the response headers set by then.
type gzipResponseWriter struct {
	gin.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

func (w *gzipResponseWriter) decide(firstChunk int) {
	if w.decided {
		return
	}
	w.decided = true
	h := w.Header()
	if h.Get("Content-Encoding") != "" || firstChunk < gzipMinBytes || !compressibleType(h.Get("Content-Type")) {
		return
	}
	switch w.Status() {
	case http.StatusNoContent, http.StatusNotModified:
		return
	}
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	gz := gzipWriterPool.Get().(*gzip.Writer)
	gz.Reset(w.ResponseWriter)
	w.gz = gz
}

// Write compresses data when the response was chosen for compression.
func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	w.decide(len(data))
	if w.gz != nil {
		return w.gz.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

// WriteString compresses s when the response was chosen for compression.
func (w *gzipResponseWriter) WriteString(s string) (int, error) {
	w.decide(len(s))
	if w.gz != nil {
		return io.WriteString(w.gz, s)
	}
	return w.ResponseWriter.WriteString(s)
}

// Flush flushes buffered compressed data before flushing the connection.
func (w *gzipResponseWriter) Flush() {
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	w.ResponseWriter.Flush()
}

func (w *gzipResponseWriter) close() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	w.gz.Reset(nil)
	gzipWriterPool.Put(w.gz)
	w.gz = nil
}

// Gzip compresses JSON and text responses for clients that accept gzip.
// Responses that already carry a Content-Encoding, binary content types
// (attachment downloads) and small bodies are passed through untouched.
func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodHead ||
			!strings.Contains(strings.ToLower(c.GetHeader("Accept-Encoding")), "gzip") {
			c.Next()
			return
		}
		c.Writer.Header().Add("Vary", "Accept-Encoding")
		gw := &gzipResponseWriter{ResponseWriter: c.Writer}
		c.Writer = gw
		defer func() {
			gw.close()
			c.Writer = gw.ResponseWriter
		}()
		c.Next()
	}
}
