package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/internal/db"
)

const contactLimitNumberWidth = 10

func TestPhoneContactsImportRejectsMoreThanBatchLimit(t *testing.T) {
	if err := db.Init(filepath.Join(t.TempDir(), "phone_contacts_limit.db")); err != nil {
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

	var vcf strings.Builder
	for index := 0; index <= maxContactBatch; index++ {
		vcf.WriteString("BEGIN:VCARD\nVERSION:3.0\nFN:Contact\nTEL:")
		vcf.WriteString(contactLimitNumber(index))
		vcf.WriteString("\nEND:VCARD\n")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/phone/contacts/import", bytes.NewReader(multipartVCF(t, vcf.String())))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=testhideck")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"batch_too_large"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	rows, err := db.ListPhoneContacts(t.Context())
	if err != nil || len(rows) != 0 {
		t.Fatalf("oversized import persisted rows=%d err=%v", len(rows), err)
	}
}

func contactLimitNumber(index int) string {
	value := strconv.Itoa(index)
	return strings.Repeat("0", contactLimitNumberWidth-len(value)) + value
}
