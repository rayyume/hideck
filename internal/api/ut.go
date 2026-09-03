package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vowifi-go/runtimehost"
)

var (
	errUtUnavailable = errors.New("Ut/XCAP is unavailable")
	errUtXCAPPDN     = errors.New("XCAP PDN is not established")
	errUtIdentity    = errors.New("IMS public identity is not registered")
	errUtManyChanges = errors.New("one Ut request may change only one service")
)

type utIdentity struct {
	XUI      string
	Fallback []string
}

type utDocumentClient interface {
	Get(context.Context, string, []string) (runtimehost.UtDocument, error)
	Put(context.Context, runtimehost.UtDocument) (runtimehost.UtDocument, error)
}

type utClientFunc func(string) (utDocumentClient, utIdentity, error)

type utView struct {
	XUI                    string         `json:"xui"`
	ETag                   string         `json:"etag"`
	CommunicationDiversion utToggle       `json:"communication_diversion"`
	IdentityRestriction    utIdentityView `json:"identity_restriction"`
	IncomingBarring        utToggle       `json:"incoming_barring"`
	OutgoingBarring        utToggle       `json:"outgoing_barring"`
}

type utToggle struct {
	Active bool   `json:"active"`
	Target string `json:"target,omitempty"`
}

type utIdentityView struct {
	Active     bool `json:"active"`
	Restricted bool `json:"restricted"`
}

type utPatchRequest struct {
	ETag                   string          `json:"etag"`
	CommunicationDiversion *utToggle       `json:"communication_diversion"`
	IdentityRestriction    *utIdentityView `json:"identity_restriction"`
	IncomingBarring        *utToggle       `json:"incoming_barring"`
	OutgoingBarring        *utToggle       `json:"outgoing_barring"`
}

func (s *Server) registerUtRoutes(api *gin.RouterGroup) {
	api.GET("/devices/:device_id/ut/simservs", s.handleUtGet)
	api.PUT("/devices/:device_id/ut/simservs", s.handleUtPut)
}

func (s *Server) handleUtGet(c *gin.Context) {
	client, ident, err := s.lookupUtClient(deviceIDParam(c))
	if err != nil {
		writeUtError(c, err)
		return
	}
	doc, err := client.Get(c.Request.Context(), ident.XUI, ident.Fallback)
	if err != nil {
		writeUtError(c, err)
		return
	}
	c.JSON(http.StatusOK, utViewFromDocument(doc))
}

func (s *Server) handleUtPut(c *gin.Context) {
	var req utPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "invalid JSON"})
		return
	}
	if n := req.changeCount(); n != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": errUtManyChanges.Error()})
		return
	}
	client, ident, err := s.lookupUtClient(deviceIDParam(c))
	if err != nil {
		writeUtError(c, err)
		return
	}
	doc, err := client.Get(c.Request.Context(), ident.XUI, ident.Fallback)
	if err != nil {
		writeUtError(c, err)
		return
	}
	if strings.TrimSpace(req.ETag) == "" || req.ETag != doc.ETag {
		c.JSON(http.StatusPreconditionFailed, gin.H{"status": "error", "message": runtimehost.ErrUtPrecondition.Error()})
		return
	}
	req.apply(&doc)
	saved, err := client.Put(c.Request.Context(), doc)
	if err != nil {
		writeUtError(c, err)
		return
	}
	c.JSON(http.StatusOK, utViewFromDocument(saved))
}

func (s *Server) lookupUtClient(deviceID string) (utDocumentClient, utIdentity, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, utIdentity{}, errUtIdentity
	}
	if s != nil && s.utClient != nil {
		return s.utClient(deviceID)
	}
	if s == nil || s.pool == nil {
		return nil, utIdentity{}, errUtXCAPPDN
	}
	inst := s.pool.GetVoWiFiAppForDevice(deviceID)
	if inst == nil {
		return nil, utIdentity{}, errUtXCAPPDN
	}
	access, err := inst.UtAccess()
	if err != nil {
		if strings.Contains(err.Error(), "public identity") {
			return nil, utIdentity{}, errUtIdentity
		}
		return nil, utIdentity{}, errUtXCAPPDN
	}
	return access.Client, utIdentity{XUI: access.XUI, Fallback: access.More}, nil
}

func (req utPatchRequest) changeCount() int {
	n := 0
	if req.CommunicationDiversion != nil {
		n++
	}
	if req.IdentityRestriction != nil {
		n++
	}
	if req.IncomingBarring != nil {
		n++
	}
	if req.OutgoingBarring != nil {
		n++
	}
	return n
}

func (req utPatchRequest) apply(doc *runtimehost.UtDocument) {
	if req.CommunicationDiversion != nil {
		doc.SetCommunicationDiversion(req.CommunicationDiversion.Active, req.CommunicationDiversion.Target)
	}
	if req.IdentityRestriction != nil {
		doc.SetIdentityRestriction(req.IdentityRestriction.Active, req.IdentityRestriction.Restricted)
	}
	if req.IncomingBarring != nil || req.OutgoingBarring != nil {
		in := doc.IncomingBarring.Active
		out := doc.OutgoingBarring.Active
		if req.IncomingBarring != nil {
			in = req.IncomingBarring.Active
		}
		if req.OutgoingBarring != nil {
			out = req.OutgoingBarring.Active
		}
		doc.SetBarring(in, out)
	}
}

func utViewFromDocument(doc runtimehost.UtDocument) utView {
	view := utView{XUI: doc.XUI, ETag: doc.ETag}
	view.CommunicationDiversion = utToggle{
		Active: doc.CommunicationDiversion.Active, Target: doc.CommunicationDiversion.Target,
	}
	view.IdentityRestriction = utIdentityView{
		Active: doc.IdentityRestriction.Active, Restricted: doc.IdentityRestriction.Restricted,
	}
	view.IncomingBarring = utToggle{Active: doc.IncomingBarring.Active}
	view.OutgoingBarring = utToggle{Active: doc.OutgoingBarring.Active}
	return view
}

func writeUtError(c *gin.Context, err error) {
	c.JSON(utStatus(err), gin.H{"status": "error", "message": utPublicMessage(err)})
}

func utPublicMessage(err error) string {
	switch {
	case errors.Is(err, runtimehost.ErrUtDocumentNotFound):
		return "运营商没有这份补充业务文档"
	case errors.Is(err, runtimehost.ErrUtPrecondition):
		return "补充业务已被其他请求更新，请刷新后重试"
	case errors.Is(err, errUtManyChanges):
		return errUtManyChanges.Error()
	case errors.Is(err, errUtIdentity):
		return "IMS 未注册，无法读取补充业务"
	case errors.Is(err, errUtXCAPPDN), errors.Is(err, errUtUnavailable):
		return "XCAP 承载未建立"
	case errors.Is(err, runtimehost.ErrUtUnavailable):
		if strings.Contains(err.Error(), "timed out") {
			return "运营商 Ut/XCAP 超时：补充业务服务器在运营商内网，当前 IMS 隧道连不上。"
		}
		return "运营商 Ut/XCAP 不可用"
	default:
		return "补充业务请求失败"
	}
}

func utStatus(err error) int {
	switch {
	case errors.Is(err, runtimehost.ErrUtDocumentNotFound):
		return http.StatusNotFound
	case errors.Is(err, runtimehost.ErrUtPrecondition):
		return http.StatusPreconditionFailed
	case errors.Is(err, errUtManyChanges):
		return http.StatusBadRequest
	case errors.Is(err, errUtIdentity), errors.Is(err, errUtXCAPPDN), errors.Is(err, errUtUnavailable):
		return http.StatusConflict
	case errors.Is(err, runtimehost.ErrUtUnavailable):
		return http.StatusBadGateway
	default:
		return http.StatusBadGateway
	}
}
