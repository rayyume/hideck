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
	c.JSON(http.StatusOK, db.LookupPhoneIdentity(c.Request.Context(), number))
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
	c.JSON(http.StatusOK, gin.H{"contacts": rows})
}

type phoneContactRequest struct {
	Number string `json:"number"`
	Name   string `json:"name"`
}

func (s *Server) handlePhoneContactsUpsert(c *gin.Context) {
	var req phoneContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "invalid_json", "message": "请求格式不正确"})
		return
	}
	row, err := db.UpsertPhoneContact(c.Request.Context(), req.Number, req.Name)
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
	if err := db.DeletePhoneContact(c.Request.Context(), number); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "code": "contact_delete_failed", "message": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
