package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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
	var importedIDs []string
	for _, contact := range payload.Contacts {
		if contact["name"] == "张三" {
			importedIDs = append(importedIDs, contact["contact_id"].(string))
		}
	}
	if len(importedIDs) != 2 || importedIDs[0] == "" || importedIDs[0] != importedIDs[1] {
		t.Fatalf("multi-number import contact IDs = %v", importedIDs)
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

func TestPhoneContactsPagination(t *testing.T) {
	if err := db.Init(filepath.Join(t.TempDir(), "phone_contacts_page.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB = nil })
	for index, number := range []string{"10000", "10001", "10002"} {
		if _, err := db.UpsertPhoneContact(t.Context(), number, "联系人"+strconv.Itoa(index)); err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	server := &Server{auth: config.WebConfig{Username: "admin", Password: "secret"}}
	router := gin.New()
	api := router.Group("/api")
	api.Use(server.authMiddleware())
	server.registerPhoneContactRoutes(api)
	token := testSessionToken(t, "secret", time.Now().Add(time.Hour))

	first := requestContactsPage(t, router, token, "/api/phone/contacts?limit=2&offset=0")
	if len(first.Contacts) != 2 || first.Total != 3 || first.NextOffset != 2 || !first.HasMore {
		t.Fatalf("unexpected first contacts page: %+v", first)
	}
	last := requestContactsPage(t, router, token, "/api/phone/contacts?limit=2&offset=2")
	if len(last.Contacts) != 1 || last.Total != 3 || last.NextOffset != 3 || last.HasMore {
		t.Fatalf("unexpected last contacts page: %+v", last)
	}

	invalid := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/phone/contacts?limit=201", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(invalid, req)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid pagination status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestPhoneContactsBatchReturnsOneAtomicGroup(t *testing.T) {
	if err := db.Init(filepath.Join(t.TempDir(), "phone_contacts_batch.db")); err != nil {
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

	body, _ := json.Marshal(map[string]any{"atomic": true, "contacts": []map[string]string{
		{"number": "10000", "name": "客服", "group_key": "manual"},
		{"number": "10001", "name": "客服", "group_key": "manual"},
	}})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/phone/contacts/batch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("batch status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Imported int              `json:"imported"`
		Contacts []map[string]any `json:"contacts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || len(result.Contacts) != 2 ||
		result.Contacts[0]["contact_id"] == "" ||
		result.Contacts[0]["contact_id"] != result.Contacts[1]["contact_id"] {
		t.Fatalf("batch result = %+v", result)
	}
}

func TestPhoneContactsBatchKeepsPartialSuccessByDefault(t *testing.T) {
	router, token := newPhoneContactsTestRouter(t, "phone_contacts_partial_batch.db")
	body, _ := json.Marshal(map[string]any{"contacts": []map[string]string{
		{"number": "10000", "name": "有效"},
		{"number": "invalid", "name": "无效"},
	}})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/phone/contacts/batch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("batch status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Skipped != 1 {
		t.Fatalf("batch result = %+v", result)
	}
}

func TestPhoneContactGroupRoutesAffectUnloadedNumbers(t *testing.T) {
	router, token := newPhoneContactsTestRouter(t, "phone_contacts_group_routes.db")
	rows, err := db.UpsertPhoneContactBatchWithRegion(t.Context(), []db.PhoneContactInput{
		{Number: "10000", Name: "旧名字", GroupKey: "person"},
		{Number: "10001", Name: "旧名字", GroupKey: "person"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	updateBody, _ := json.Marshal(map[string]string{"contact_id": rows[0].ContactID, "name": "新名字"})
	updated := requestPhoneContactMutation(t, phoneContactMutationRequest{
		Router: router, Token: token, Method: http.MethodPut,
		Body: updateBody, Target: "/api/phone/contacts/group",
	})
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte("新名字")) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	deleted := requestPhoneContactMutation(t, phoneContactMutationRequest{
		Router: router, Token: token, Method: http.MethodDelete,
		Target: "/api/phone/contacts/group?contact_id=" + rows[0].ContactID,
	})
	if deleted.Code != http.StatusOK || !bytes.Contains(deleted.Body.Bytes(), []byte(`"deleted":2`)) {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func newPhoneContactsTestRouter(t *testing.T, databaseName string) (http.Handler, string) {
	t.Helper()
	previous := db.DB
	if err := db.Init(filepath.Join(t.TempDir(), databaseName)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB = previous })
	gin.SetMode(gin.TestMode)
	server := &Server{auth: config.WebConfig{Username: "admin", Password: "secret"}}
	router := gin.New()
	api := router.Group("/api")
	api.Use(server.authMiddleware())
	server.registerPhoneContactRoutes(api)
	return router, testSessionToken(t, "secret", time.Now().Add(time.Hour))
}

type phoneContactMutationRequest struct {
	Router http.Handler
	Token  string
	Method string
	Body   []byte
	Target string
}

func requestPhoneContactMutation(t *testing.T, input phoneContactMutationRequest) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(input.Method, input.Target, bytes.NewReader(input.Body))
	req.Header.Set("Authorization", "Bearer "+input.Token)
	req.Header.Set("Content-Type", "application/json")
	input.Router.ServeHTTP(recorder, req)
	return recorder
}

type contactsPageResponse struct {
	Contacts   []map[string]any `json:"contacts"`
	Total      int              `json:"total"`
	NextOffset int              `json:"next_offset"`
	HasMore    bool             `json:"has_more"`
}

func requestContactsPage(t *testing.T, router http.Handler, token, target string) contactsPageResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("contacts page status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var page contactsPageResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
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
