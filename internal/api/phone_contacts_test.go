package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/internal/db"
)

func TestPhoneLookupAndContacts(t *testing.T) {
	if err := db.Init(filepath.Join(t.TempDir(), "phone_contacts.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB = nil })

	gin.SetMode(gin.TestMode)
	server := &Server{auth: config.WebConfig{Username: "admin", Password: "secret"}}
	router := gin.New()
	api := router.Group("/api")
	api.Use(server.authMiddleware())
	server.registerPhoneContactRoutes(api)
	token := testSessionToken(t, "secret", time.Now().Add(time.Hour))

	lookup := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/phone/lookup?number=10086", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(lookup, req)
	if lookup.Code != http.StatusOK {
		t.Fatalf("lookup status=%d body=%s", lookup.Code, lookup.Body.String())
	}
	var ident map[string]any
	if err := json.Unmarshal(lookup.Body.Bytes(), &ident); err != nil {
		t.Fatal(err)
	}
	if ident["carrier"] != "中国移动" || ident["kind"] != "service" {
		t.Fatalf("%v", ident)
	}

	save := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"number": "10086", "name": "移动客服"})
	req = httptest.NewRequest(http.MethodPut, "/api/phone/contacts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(save, req)
	if save.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", save.Code, save.Body.String())
	}
	if err := json.Unmarshal(save.Body.Bytes(), &ident); err != nil {
		t.Fatal(err)
	}
	if ident["name"] != "移动客服" || ident["title"] != "移动客服" {
		t.Fatalf("%v", ident)
	}
}
