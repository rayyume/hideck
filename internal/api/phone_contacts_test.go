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

	list := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/phone/contacts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var payload struct {
		Contacts []map[string]any `json:"contacts"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Contacts) != 1 || payload.Contacts[0]["name"] != "移动客服" || payload.Contacts[0]["carrier"] != "中国移动" {
		t.Fatalf("%v", payload)
	}

	vcf := "BEGIN:VCARD\nVERSION:3.0\nFN:张三\nTEL;TYPE=CELL:13800138000\nTEL;TYPE=CELL:18600001111\nEND:VCARD\n"
	importReq := httptest.NewRequest(http.MethodPost, "/api/phone/contacts/import", bytes.NewReader(multipartVCF(t, vcf)))
	importReq.Header.Set("Authorization", "Bearer "+token)
	importReq.Header.Set("Content-Type", "multipart/form-data; boundary=testhideck")
	imported := httptest.NewRecorder()
	router.ServeHTTP(imported, importReq)
	if imported.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%s", imported.Code, imported.Body.String())
	}
	var importResult struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
		Parsed   int `json:"parsed"`
	}
	if err := json.Unmarshal(imported.Body.Bytes(), &importResult); err != nil {
		t.Fatal(err)
	}
	if importResult.Imported != 2 || importResult.Parsed != 2 {
		t.Fatalf("%+v %s", importResult, imported.Body.String())
	}

	list = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/phone/contacts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(list, req)
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Contacts) != 3 {
		t.Fatalf("want 3 contacts after multi-number import, got %v", payload)
	}

	exported := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/phone/contacts/export", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(exported, req)
	if exported.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", exported.Code, exported.Body.String())
	}
	bodyText := exported.Body.String()
	if !bytes.Contains(exported.Body.Bytes(), []byte("13800138000")) ||
		!bytes.Contains(exported.Body.Bytes(), []byte("18600001111")) ||
		!bytes.Contains(exported.Body.Bytes(), []byte("BEGIN:VCARD")) {
		t.Fatalf("export missing multi TEL: %s", bodyText)
	}

	del := httptest.NewRecorder()
	delBody, _ := json.Marshal(map[string]any{"numbers": []string{"13800138000", "18600001111"}})
	req = httptest.NewRequest(http.MethodPost, "/api/phone/contacts/delete", bytes.NewReader(delBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(del, req)
	if del.Code != http.StatusOK {
		t.Fatalf("batch delete status=%d body=%s", del.Code, del.Body.String())
	}
	var deleted struct {
		Deleted int `json:"deleted"`
	}
	if err := json.Unmarshal(del.Body.Bytes(), &deleted); err != nil {
		t.Fatal(err)
	}
	if deleted.Deleted != 2 {
		t.Fatalf("%+v", deleted)
	}
}

func multipartVCF(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("--testhideck\r\n")
	buf.WriteString(`Content-Disposition: form-data; name="file"; filename="xiaomi.vcf"` + "\r\n")
	buf.WriteString("Content-Type: text/vcard\r\n\r\n")
	buf.WriteString(body)
	buf.WriteString("\r\n--testhideck--\r\n")
	return buf.Bytes()
}
