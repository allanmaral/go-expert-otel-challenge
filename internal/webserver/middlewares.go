package webserver

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// Ported from Chi's middleware, source:
// https://github.com/go-chi/chi/blob/master/middleware/request_id.go
type ctxKeyRequestID int

const RequestIDKey ctxKeyRequestID = 0

var RequestIDHeader = "X-Request-Id"
var prefix string
var reqid uint64

func init() {
	hostname, err := os.Hostname()
	if hostname == "" || err != nil {
		hostname = "localhost"
	}
	var buffer [12]byte
	var b64 string
	for len(b64) < 10 {
		rand.Read(buffer[:])
		b64 = base64.StdEncoding.EncodeToString(buffer[:])
		b64 = strings.NewReplacer("+", "", "/", "").Replace(b64)
	}
	prefix = fmt.Sprintf("%s/%s", hostname, b64[0:10])
}

func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			newId := atomic.AddUint64(&reqid, 1)
			requestID = fmt.Sprintf("%s-%06d", prefix, newId)
		}
		ctx = context.WithValue(ctx, RequestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, ok := ctx.Value(RequestIDKey).(string)
	if !ok {
		return ""
	}
	return requestID
}

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterWrapper) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func WithLogging(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &responseWriterWrapper{w, http.StatusOK}
		next.ServeHTTP(rw, r)

		elapsed := time.Since(start)

		requestID := GetRequestID(r.Context())
		if requestID != "" {
			requestID = fmt.Sprintf("[%s] ", requestID)
		}

		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		// [888d26e39339/tKgftQxacB-000039] "GET http://goapp:8080/metrics HTTP/1.1" from 172.18.0.3:41954 - 200 1395B in 2.76975ms
		logger.Printf("%s\"%s %s://%s%s %s\" from %s - %d in %s", requestID, r.Method, scheme, r.Host, r.URL.String(), r.Proto, r.RemoteAddr, rw.statusCode, elapsed.String())
	})
}
