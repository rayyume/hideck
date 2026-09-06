package api

import (
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

func TestSMSMessageTargetHTTP(t *testing.T) {
	previous := db.DB
	if err := db.Init(filepath.Join(t.TempDir(), "message-target.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB = previous })
	message := db.SMS{ID: 7, ICCID: "card-a", IMSI: "shared", Peer: "sender", Type: 1, Timestamp: time.Now()}
	if err := db.DB.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	contact := db.SMSContact{ICCID: message.ICCID, IMSI: message.IMSI, Peer: message.Peer, UnreadCount: 1}
	if err := db.DB.Create(&contact).Error; err != nil {
		t.Fatal(err)
	}
	server := &Server{auth: config.WebConfig{Username: "admin", Password: "secret"}}
	router := gin.New()
	router.Group("/api", server.authMiddleware()).GET("/sms/messages/:id", server.handleSMSMessageTarget)
	token := testSessionToken(t, "secret", time.Now().Add(time.Hour))
	for _, sample := range []struct {
		id   string
		auth bool
		want int
	}{
		{"7", false, http.StatusUnauthorized},
		{"0", true, http.StatusBadRequest},
		{"-1", true, http.StatusBadRequest},
		{"1junk", true, http.StatusBadRequest},
		{"9007199254740992", true, http.StatusBadRequest},
		{"99", true, http.StatusNotFound},
		{"7", true, http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/sms/messages/"+sample.id, nil)
		if sample.auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != sample.want {
			t.Fatalf("id=%s auth=%t status=%d body=%s", sample.id, sample.auth, response.Code, response.Body)
		}
		if sample.want == http.StatusOK {
			assertSMSMessageTargetResponse(t, response)
		}
	}
}

func assertSMSMessageTargetResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var target db.SMSMessageTarget
	if err := json.Unmarshal(response.Body.Bytes(), &target); err != nil {
		t.Fatal(err)
	}
	if target.Message.ID != 7 || target.Contact.ICCID != "card-a" || len(target.Messages) != 1 || target.Messages[0].ID != 7 {
		t.Fatalf("unexpected target: %+v", target)
	}
	if target.HasMore || target.Contact.UnreadCount != 1 || target.Message.Status != 0 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected state/cache: %+v", target)
	}
}
