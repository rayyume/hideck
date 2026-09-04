package api

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yibaiba/hideck/internal/db"
	"github.com/yibaiba/hideck/internal/phonebook"
	"gorm.io/gorm"
)

const maxContactImportBytes = 8 << 20
const maxContactBatch = 5000
const maxContactPageSize = 200

type contactPageRequest struct {
	Limit     int
	Offset    int
	Paginated bool
}

func (s *Server) registerPhoneContactRoutes(api *gin.RouterGroup) {
	api.GET("/phone/lookup", s.handlePhoneLookup)
	api.GET("/phone/contacts", s.handlePhoneContactsList)
	api.PUT("/phone/contacts", s.handlePhoneContactsUpsert)
	api.DELETE("/phone/contacts", s.handlePhoneContactsDelete)
	api.PUT("/phone/contacts/group", s.handlePhoneContactGroupUpdate)
	api.DELETE("/phone/contacts/group", s.handlePhoneContactGroupDelete)
	api.POST("/phone/contacts/batch", s.handlePhoneContactsBatch)
	api.POST("/phone/contacts/delete", s.handlePhoneContactsBatchDelete)
	api.GET("/phone/contacts/export", s.handlePhoneContactsExport)
	api.POST("/phone/contacts/import", s.handlePhoneContactsImport)
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
	page, err := contactPageParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "invalid_pagination", "message": err.Error()})
		return
	}
	rows, total, err := loadPhoneContacts(c, page)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "code": "contacts_list_failed", "message": err.Error()})
		return
	}
	if rows == nil {
		rows = []db.PhoneContact{}
	}
	out := phoneContactIdentities(rows)
	c.JSON(http.StatusOK, gin.H{
		"contacts":    out,
		"total":       total,
		"offset":      page.Offset,
		"next_offset": page.Offset + len(rows),
		"has_more":    page.Paginated && int64(page.Offset+len(rows)) < total,
	})
}

func contactPageParams(c *gin.Context) (contactPageRequest, error) {
	rawLimit, paginated := c.GetQuery("limit")
	if !paginated {
		return contactPageRequest{}, nil
	}
	limit, err := strconv.Atoi(rawLimit)
	if err != nil || limit < 1 || limit > maxContactPageSize {
		return contactPageRequest{}, errors.New("limit 必须是 1 到 200")
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		return contactPageRequest{}, errors.New("offset 不能小于 0")
	}
	return contactPageRequest{Limit: limit, Offset: offset, Paginated: true}, nil
}

func loadPhoneContacts(c *gin.Context, page contactPageRequest) ([]db.PhoneContact, int64, error) {
	if !page.Paginated {
		rows, err := db.ListPhoneContacts(c.Request.Context())
		return rows, int64(len(rows)), err
	}
	total, err := db.CountPhoneContacts(c.Request.Context())
	if err != nil {
		return nil, 0, err
	}
	rows, err := db.ListPhoneContactsPage(c.Request.Context(), page.Limit, page.Offset)
	return rows, total, err
}

type phoneContactRequest struct {
	Number    string `json:"number"`
	Name      string `json:"name"`
	DeviceID  string `json:"device_id"`
	ContactID string `json:"contact_id"`
	GroupKey  string `json:"group_key"`
}

func (s *Server) handlePhoneContactsUpsert(c *gin.Context) {
	var req phoneContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "invalid_json", "message": "请求格式不正确"})
		return
	}
	region := s.phoneNumberRegion(req.DeviceID)
	row, err := db.UpsertPhoneContactWithRegion(c.Request.Context(), db.PhoneContactInput{
		Number: req.Number, Name: req.Name, Region: region, ContactID: req.ContactID,
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

type phoneContactDeleteRequest struct {
	DeviceID string   `json:"device_id"`
	Numbers  []string `json:"numbers"`
}

func phoneContactIdentities(rows []db.PhoneContact) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, db.PhoneContactIdentity(row))
	}
	return out
}

func (s *Server) handlePhoneContactsBatchDelete(c *gin.Context) {
	var req phoneContactDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "invalid_json", "message": "请求格式不正确"})
		return
	}
	deleted, err := db.DeletePhoneContactsWithRegion(c.Request.Context(), req.Numbers, s.phoneNumberRegion(req.DeviceID))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, db.ErrInvalidPhoneContact) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"status": "error", "code": "contact_delete_failed", "message": "批量删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func (s *Server) handlePhoneContactsExport(c *gin.Context) {
	rows, err := db.ListPhoneContacts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "code": "contacts_export_failed", "message": err.Error()})
		return
	}
	contacts := make([]phonebook.Contact, 0, len(rows))
	for _, row := range rows {
		contacts = append(contacts, phonebook.Contact{ContactID: row.ContactID, Name: row.Name, Number: row.Number})
	}
	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	stamp := time.Now().Format("20060102")
	if format == "csv" {
		c.Header("Content-Disposition", `attachment; filename="hideck-contacts-`+stamp+`.csv"`)
		c.Data(http.StatusOK, "text/csv; charset=utf-8", phonebook.ExportCSV(contacts))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="hideck-contacts-`+stamp+`.vcf"`)
	c.Data(http.StatusOK, "text/vcard; charset=utf-8", phonebook.ExportVCard(contacts))
}

func (s *Server) handlePhoneContactsImport(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "missing_file", "message": "请选择要导入的联系人文件"})
		return
	}
	if file.Size > maxContactImportBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"status": "error", "code": "file_too_large", "message": "导入文件不能超过 8MB"})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "open_file_failed", "message": "无法读取导入文件"})
		return
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, maxContactImportBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "read_file_failed", "message": "无法读取导入文件"})
		return
	}
	parsed := phonebook.Parse(data, filepath.Base(file.Filename))
	if len(parsed) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "code": "empty_import", "message": "文件里没有识别到联系人。请用手机或 Google 通讯录导出的 vcf / csv（iOS、Google、三星、小米、华为、OPPO、vivo 都可以）"})
		return
	}
	if len(parsed) > maxContactBatch {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error", "code": "batch_too_large", "message": "一次最多导入 5000 条，请拆分文件后重试",
		})
		return
	}
	region := s.phoneNumberRegion(c.PostForm("device_id"))
	inputs := make([]db.PhoneContactInput, 0, len(parsed))
	for _, item := range parsed {
		inputs = append(inputs, db.PhoneContactInput{
			Number: item.Number, Name: item.Name, Region: region, GroupKey: item.ContactID,
		})
	}
	imported, skipped, err := db.UpsertPhoneContactsWithRegion(c.Request.Context(), inputs, region)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "code": "contact_import_failed", "message": "导入失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported, "skipped": skipped, "parsed": len(parsed)})
}
