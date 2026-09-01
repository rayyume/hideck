package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vowifi-go/xcap"
	"github.com/yibaiba/hideck/internal/config"
)

func TestUtRoutesRequireAuthAndRejectMultiChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{auth: config.WebConfig{Username: "admin", Password: "secret"}}
	router := gin.New()
	api := router.Group("/api")
	api.Use(server.authMiddleware())
	server.registerUtRoutes(api)

	unauth := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-1/ut/simservs", nil)
	router.ServeHTTP(unauth, req)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth = %d", unauth.Code)
	}

	token := testSessionToken(t, "secret", time.Now().Add(time.Hour))
	body := `{"etag":"v1","communication_diversion":{"active":true},"incoming_barring":{"active":true}}`
	put := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/devices/dev-1/ut/simservs", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(put, req)
	if put.Code != http.StatusBadRequest || !strings.Contains(put.Body.String(), "only one service") {
		t.Fatalf("multi change = %d %s", put.Code, put.Body.String())
	}
}

func TestUtGetAndPutUseRealXCAPDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var lastMatch string
	docXML := `<simservs xmlns="http://uri.etsi.org/ngn/params/xml/simservs/xcap"><communication-diversion active="false"></communication-diversion></simservs>`
	xcapServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write([]byte(docXML))
		case http.MethodPut:
			lastMatch = r.Header.Get("If-Match")
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte("tel:+441234")) {
				t.Fatalf("put body = %s", body)
			}
			w.Header().Set("ETag", `"v2"`)
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(xcapServer.Close)
	client := &xcap.Client{HTTP: xcapServer.Client(), Host: strings.TrimPrefix(xcapServer.URL, "http://")}
	client.HTTP.Transport = utRewriteHTTPS(xcapServer)
	server := &Server{
		auth: config.WebConfig{Username: "admin", Password: "secret"},
		utClient: func(deviceID string) (*xcap.Client, utIdentity, error) {
			if deviceID != "dev-1" {
				t.Fatalf("device = %s", deviceID)
			}
			return client, utIdentity{XUI: "sip:user@ims.example"}, nil
		},
	}
	router := gin.New()
	api := router.Group("/api")
	api.Use(server.authMiddleware())
	server.registerUtRoutes(api)
	token := testSessionToken(t, "secret", time.Now().Add(time.Hour))

	get := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-1/ut/simservs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(get, req)
	if get.Code != http.StatusOK {
		t.Fatalf("get = %d %s", get.Code, get.Body.String())
	}
	var view utView
	if err := json.Unmarshal(get.Body.Bytes(), &view); err != nil || view.ETag != "v1" {
		t.Fatalf("view = %+v err=%v", view, err)
	}

	putBody, _ := json.Marshal(utPatchRequest{
		ETag: "v1", CommunicationDiversion: &utToggle{Active: true, Target: "tel:+441234"},
	})
	put := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/devices/dev-1/ut/simservs", bytes.NewReader(putBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(put, req)
	if put.Code != http.StatusOK || lastMatch != `"v1"` {
		t.Fatalf("put = %d match=%q body=%s", put.Code, lastMatch, put.Body.String())
	}
	if err := json.Unmarshal(put.Body.Bytes(), &view); err != nil || view.ETag != "v2" || view.CommunicationDiversion.Target != "tel:+441234" {
		t.Fatalf("saved = %+v err=%v", view, err)
	}
}

func TestUtGetWithoutClientReportsMissingPDN(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{auth: config.WebConfig{Username: "admin", Password: "secret"}}
	router := gin.New()
	api := router.Group("/api")
	api.Use(server.authMiddleware())
	server.registerUtRoutes(api)
	token := testSessionToken(t, "secret", time.Now().Add(time.Hour))
	get := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-1/ut/simservs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(get, req)
	if get.Code != http.StatusConflict || !strings.Contains(get.Body.String(), "XCAP 承载未建立") {
		t.Fatalf("missing PDN = %d %s", get.Code, get.Body.String())
	}
}

func TestUtPublicMessageHidesXCAPURL(t *testing.T) {
	err := fmt.Errorf("%w: timed out", xcap.ErrUnavailable)
	got := utPublicMessage(err)
	if strings.Contains(got, "sip:") || strings.Contains(got, "https://") || strings.Contains(got, "23415") {
		t.Fatalf("message = %q", got)
	}
	if !strings.Contains(got, "超时") {
		t.Fatalf("message = %q", got)
	}
}

func utRewriteHTTPS(server *httptest.Server) http.RoundTripper {
	base := server.Client().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return base.RoundTrip(clone)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
