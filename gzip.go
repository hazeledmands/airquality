package main

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

var gzipWriters = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// acceptsGzip reports whether the request allows a gzip response. It honours
// an explicit "gzip;q=0".
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(name), "gzip") {
			q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
			return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
		}
	}
	return false
}

type gzipResponseWriter struct {
	http.ResponseWriter
	zw *gzip.Writer
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	g.Header().Del("Content-Length")
	return g.zw.Write(b)
}

// withGzip compresses responses for clients that accept gzip. Nothing in
// front of the app compresses (the ingress passes bodies through), and the
// dashboard JSON shrinks about 10x.
func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		zw := gzipWriters.Get().(*gzip.Writer)
		zw.Reset(w)
		defer func() {
			zw.Close()
			gzipWriters.Put(zw)
		}()
		w.Header().Set("Content-Encoding", "gzip")
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, zw: zw}, r)
	})
}
