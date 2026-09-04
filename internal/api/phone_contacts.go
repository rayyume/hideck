package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yibaiba/hideck/internal/db"
	"gorm.io/gorm"
)

func (s *Server) registerPhoneContactRoutes(api *gin.RouterGroup) {
	api.GET("/phone/lookup", s.handlePhoneLookup)
	api.GET("/phone/contacts", s.handlePhoneContactsList)
	api.PUT("/phone/contacts", s.handlePhoneContactsUpsert)
	api.DELETE("/phone/contacts", s.handlePhoneContactsDelete)
}

func (s *Server) handlePhoneLookup(c *gin.Context) {
	number := strings.TrimSpace(c.Query("number"))
	if number == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "missing_number", "message": "缺少号码"})
		return
	}
	region := s.phoneNumberRegion(c.Query("device_id"))
	c.JSON(http.StatusOK, db.LookupPhoneIdentityWithRegion(c.Request.Context(), number, region))
}

func (s *Server) handlePhoneContactsList(c *gin.Context) {
	rows, err := db.ListPhoneContacts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "code": "contacts_list_failed", "message": err.Error()})
		return
	}
	if rows == nil {
		rows = []db.PhoneContact{}
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, db.LookupPhoneIdentity(c.Request.Context(), row.Number))
	}
	c.JSON(http.StatusOK, gin.H{"contacts": out})
}

type phoneContactRequest struct {
	Number   string `json:"number"`
	Name     string `json:"name"`
	DeviceID string `json:"device_id"`
}

func (s *Server) handlePhoneContactsUpsert(c *gin.Context) {
	var req phoneContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "invalid_json", "message": "请求格式不正确"})
		return
	}
	region := s.phoneNumberRegion(req.DeviceID)
	row, err := db.UpsertPhoneContactWithRegion(c.Request.Context(), db.PhoneContactInput{
		Number: req.Number, Name: req.Name, Region: region,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, db.ErrInvalidPhoneContact) {
			status = http.StatusBadRequest
		} else if errors.Is(err, gorm.ErrInvalidDB) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"status": "error", "code": "contact_save_failed", "message": "保存联系人失败"})
		return
	}
	c.JSON(http.StatusOK, db.LookupPhoneIdentity(c.Request.Context(), row.Number))
}

func (s *Server) handlePhoneContactsDelete(c *gin.Context) {
	number := strings.TrimSpace(c.Query("number"))
	if number == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "missing_number", "message": "缺少号码"})
		return
	}
	region := s.phoneNumberRegion(c.Query("device_id"))
	if err := db.DeletePhoneContactWithRegion(c.Request.Context(), number, region); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "code": "contact_delete_failed", "message": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) phoneNumberRegion(deviceID string) string {
	if s == nil || s.pool == nil {
		return ""
	}
	return s.pool.PhoneNumberRegion(deviceID)
}
