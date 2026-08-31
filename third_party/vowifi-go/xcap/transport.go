package xcap

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// NewHTTPClient builds an HTTPS client that dials through the XCAP PDN.
func NewHTTPClient(dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	if dial == nil {
		return nil
	}
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			DialContext:       dial,
			ForceAttemptHTTP2: true,
			TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
}
