package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vowifi-go/xcap"
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

type utClientFunc func(string) (*xcap.Client, utIdentity, error)

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
		c.JSON(utStatus(err), gin.H{"status": "error", "message": err.Error()})
		return
	}
	doc, err := client.Get(c.Request.Context(), ident.XUI, ident.Fallback)
	if err != nil {
		c.JSON(utStatus(err), gin.H{"status": "error", "message": err.Error()})
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
		c.JSON(utStatus(err), gin.H{"status": "error", "message": err.Error()})
		return
	}
	doc, err := client.Get(c.Request.Context(), ident.XUI, ident.Fallback)
	if err != nil {
		c.JSON(utStatus(err), gin.H{"status": "error", "message": err.Error()})
		return
	}
	if strings.TrimSpace(req.ETag) == "" || req.ETag != doc.ETag {
		c.JSON(http.StatusPreconditionFailed, gin.H{"status": "error", "message": xcap.ErrPrecondition.Error()})
		return
	}
	req.apply(&doc)
	saved, err := client.Put(c.Request.Context(), doc)
	if err != nil {
		c.JSON(utStatus(err), gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utViewFromDocument(saved))
}

func (s *Server) lookupUtClient(deviceID string) (*xcap.Client, utIdentity, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, utIdentity{}, errUtIdentity
	}
	if s != nil && s.utClient != nil {
		return s.utClient(deviceID)
	}
	return nil, utIdentity{}, errUtXCAPPDN
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

func (req utPatchRequest) apply(doc *xcap.Document) {
	if req.CommunicationDiversion != nil {
		doc.SetCFU(req.CommunicationDiversion.Active, req.CommunicationDiversion.Target)
	}
	if req.IdentityRestriction != nil {
		doc.SetOIR(req.IdentityRestriction.Active, req.IdentityRestriction.Restricted)
	}
	if req.IncomingBarring != nil || req.OutgoingBarring != nil {
		in, out := false, false
		if doc.ICB != nil {
			in = doc.ICB.Active
		}
		if doc.OCB != nil {
			out = doc.OCB.Active
		}
		if req.IncomingBarring != nil {
			in = req.IncomingBarring.Active
		}
		if req.OutgoingBarring != nil {
			out = req.OutgoingBarring.Active
		}
		doc.SetBarring(in, out)
	}
}

func utViewFromDocument(doc xcap.Document) utView {
	view := utView{XUI: doc.XUI, ETag: doc.ETag}
	if doc.CDIV != nil {
		view.CommunicationDiversion = utToggle{Active: doc.CDIV.Active, Target: doc.CFUTarget()}
	}
	if doc.OIR != nil {
		view.IdentityRestriction = utIdentityView{Active: doc.OIR.Active, Restricted: doc.IdentityRestricted()}
	}
	if doc.ICB != nil {
		view.IncomingBarring = utToggle{Active: doc.ICB.Active}
	}
	if doc.OCB != nil {
		view.OutgoingBarring = utToggle{Active: doc.OCB.Active}
	}
	return view
}

func utStatus(err error) int {
	switch {
	case errors.Is(err, xcap.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, xcap.ErrPrecondition):
		return http.StatusPreconditionFailed
	case errors.Is(err, errUtManyChanges):
		return http.StatusBadRequest
	case errors.Is(err, errUtIdentity), errors.Is(err, errUtXCAPPDN), errors.Is(err, errUtUnavailable):
		return http.StatusConflict
	case errors.Is(err, xcap.ErrUnavailable):
		return http.StatusBadGateway
	default:
		return http.StatusBadGateway
	}
}
