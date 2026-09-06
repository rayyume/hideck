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

func TestSMSNotificationsAuthenticationAndCursorValidation(t *testing.T) {
	previous := db.DB
	if err := db.Init(filepath.Join(t.TempDir(), "notifications.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB = previous })
	server := &Server{auth: config.WebConfig{Username: "admin", Password: "secret"}}
	router := gin.New()
	group := router.Group("/api", server.authMiddleware())
	group.GET("/sms/notifications", server.handleSMSNotifications)
	token := testSessionToken(t, "secret", time.Now().Add(time.Hour))
	for _, sample := range []struct {
		query string
		auth  bool
		want  int
	}{
		{"", false, http.StatusUnauthorized},
		{"", true, http.StatusOK},
		{"?after_id=0", true, http.StatusOK},
		{"?after_id=", true, http.StatusBadRequest},
		{"?after_id=-1", true, http.StatusBadRequest},
		{"?after_id=1junk", true, http.StatusBadRequest},
		{"?after_id=1&after_id=2", true, http.StatusBadRequest},
		{"?after_id=9007199254740992", true, http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/sms/notifications"+sample.query, nil)
		if sample.auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != sample.want {
			t.Fatalf("query=%q auth=%t status=%d body=%s", sample.query, sample.auth, response.Code, response.Body)
		}
		if response.Code == http.StatusOK {
			var snapshot db.SMSNotifications
			if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.UnreadCount != 0 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected snapshot/cache header: %+v", snapshot)
			}
		}
	}
}
