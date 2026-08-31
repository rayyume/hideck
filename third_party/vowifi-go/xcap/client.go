package xcap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	simservsAUID = "simservs.ngn.etsi.org"
	maxBody      = 1 << 20
)

var (
	ErrNotFound     = errors.New("xcap: document not found")
	ErrPrecondition = errors.New("xcap: If-Match conflict")
	ErrUnavailable  = errors.New("xcap: Ut is unavailable")
)

// Transport is the HTTP round-tripper that should run over the XCAP PDN.
type Transport = http.RoundTripper

type Client struct {
	HTTP   *http.Client
	Host   string
	Domain string
}

func RootURI(host, domain, xui string) string {
	host = strings.TrimSpace(host)
	domain = strings.TrimSpace(domain)
	xui = strings.TrimSpace(xui)
	if host == "" && domain != "" {
		host = "xcap." + strings.TrimPrefix(domain, "ims.")
		if !strings.Contains(host, ".") {
			host = "xcap." + domain
		}
	}
	if host == "" || xui == "" {
		return ""
	}
	return "https://" + host + "/" + simservsAUID + "/users/" + url.PathEscape(xui) + "/simservs"
}

func (c *Client) Get(ctx context.Context, xui string, fallback []string) (Document, error) {
	candidates := uniqueXUIs(append([]string{xui}, fallback...))
	var last error
	for _, candidate := range candidates {
		doc, err := c.getOne(ctx, candidate)
		if err == nil {
			return doc, nil
		}
		last = err
		if !errors.Is(err, ErrNotFound) {
			return Document{}, err
		}
	}
	if last == nil {
		last = ErrNotFound
	}
	return Document{}, last
}

func (c *Client) Put(ctx context.Context, doc Document) (Document, error) {
	if strings.TrimSpace(doc.XUI) == "" {
		return Document{}, fmt.Errorf("xcap: missing XUI")
	}
	if strings.TrimSpace(doc.ETag) == "" {
		return Document{}, fmt.Errorf("xcap: missing If-Match etag")
	}
	body, err := doc.Marshal()
	if err != nil {
		return Document{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.documentURL(doc.XUI), bytes.NewReader(body))
	if err != nil {
		return Document{}, err
	}
	req.Header.Set("Content-Type", "application/vnd.etsi.simservs+xml")
	req.Header.Set("If-Match", `"`+doc.ETag+`"`)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		etag := resp.Header.Get("ETag")
		if etag == "" {
			etag = doc.ETag
		}
		return ParseSimservs(rawOr(raw, body), etag, doc.XUI)
	case http.StatusPreconditionFailed:
		return Document{}, ErrPrecondition
	case http.StatusNotFound:
		return Document{}, ErrNotFound
	default:
		return Document{}, fmt.Errorf("xcap: PUT status %d", resp.StatusCode)
	}
}

func (c *Client) getOne(ctx context.Context, xui string) (Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.documentURL(xui), nil)
	if err != nil {
		return Document{}, err
	}
	req.Header.Set("Accept", "application/vnd.etsi.simservs+xml, application/xml")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Document{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return ParseSimservs(raw, resp.Header.Get("ETag"), xui)
	case http.StatusNotFound:
		return Document{}, ErrNotFound
	default:
		return Document{}, fmt.Errorf("xcap: GET status %d", resp.StatusCode)
	}
}

func (c *Client) documentURL(xui string) string {
	host := ""
	domain := ""
	if c != nil {
		host, domain = c.Host, c.Domain
	}
	return RootURI(host, domain, xui)
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func uniqueXUIs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func rawOr(raw, fallback []byte) []byte {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fallback
	}
	return raw
}
