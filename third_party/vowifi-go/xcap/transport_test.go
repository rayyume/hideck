package xcap

import (
	"context"
	"net"
	"testing"
)

func TestNewHTTPClientRequiresDialer(t *testing.T) {
	if NewHTTPClient(nil) != nil {
		t.Fatal("nil dialer must not build a client")
	}
	client := NewHTTPClient(func(context.Context, string, string) (net.Conn, error) {
		return nil, net.ErrClosed
	})
	if client == nil || client.Transport == nil {
		t.Fatal("dialer client missing transport")
	}
}
