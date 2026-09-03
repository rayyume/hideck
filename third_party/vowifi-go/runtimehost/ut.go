package runtimehost

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/runtimecore"
	"github.com/iniwex5/vowifi-go/xcap"
)

var (
	ErrUtDocumentNotFound = xcap.ErrNotFound
	ErrUtPrecondition     = xcap.ErrPrecondition
	ErrUtUnavailable      = xcap.ErrUnavailable
)

type UtClientConfig struct {
	HTTP   *http.Client
	Host   string
	Domain string
	OnNet  bool
}

type UtClient struct {
	client *xcap.Client
}

type UtToggle struct {
	Active bool
	Target string
}

type UtIdentityRestriction struct {
	Active     bool
	Restricted bool
}

// UtDocument is the runtimehost representation of an ETSI simservs document.
type UtDocument struct {
	XUI                    string
	ETag                   string
	CommunicationDiversion UtToggle
	IdentityRestriction    UtIdentityRestriction
	IncomingBarring        UtToggle
	OutgoingBarring        UtToggle
	document               xcap.Document
}

// UtAccess is the UT client and public identities for one device.
type UtAccess struct {
	Client *UtClient
	XUI    string
	More   []string
}

func NewUtClient(cfg UtClientConfig) *UtClient {
	return &UtClient{client: &xcap.Client{
		HTTP: cfg.HTTP, Host: cfg.Host, Domain: cfg.Domain, OnNet: cfg.OnNet,
	}}
}

func (c *UtClient) Get(ctx context.Context, xui string, fallback []string) (UtDocument, error) {
	if c == nil || c.client == nil {
		return UtDocument{}, ErrUtUnavailable
	}
	doc, err := c.client.Get(ctx, xui, fallback)
	if err != nil {
		return UtDocument{}, err
	}
	return utDocumentFromXCAP(doc), nil
}

func (c *UtClient) Put(ctx context.Context, doc UtDocument) (UtDocument, error) {
	if c == nil || c.client == nil {
		return UtDocument{}, ErrUtUnavailable
	}
	raw := doc.document
	raw.XUI = doc.XUI
	raw.ETag = doc.ETag
	saved, err := c.client.Put(ctx, raw)
	if err != nil {
		return UtDocument{}, err
	}
	return utDocumentFromXCAP(saved), nil
}

func (doc *UtDocument) SetCommunicationDiversion(active bool, target string) {
	if doc == nil {
		return
	}
	doc.document.SetCFU(active, target)
	doc.CommunicationDiversion = UtToggle{Active: active, Target: strings.TrimSpace(target)}
}

func (doc *UtDocument) SetIdentityRestriction(active, restricted bool) {
	if doc == nil {
		return
	}
	doc.document.SetOIR(active, restricted)
	doc.IdentityRestriction = UtIdentityRestriction{Active: active, Restricted: restricted}
}

func (doc *UtDocument) SetBarring(incoming, outgoing bool) {
	if doc == nil {
		return
	}
	doc.document.SetBarring(incoming, outgoing)
	doc.IncomingBarring.Active = incoming
	doc.OutgoingBarring.Active = outgoing
}

func utDocumentFromXCAP(doc xcap.Document) UtDocument {
	result := UtDocument{XUI: doc.XUI, ETag: doc.ETag, document: doc}
	if doc.CDIV != nil {
		result.CommunicationDiversion = UtToggle{Active: doc.CDIV.Active, Target: doc.CFUTarget()}
	}
	if doc.OIR != nil {
		result.IdentityRestriction = UtIdentityRestriction{
			Active: doc.OIR.Active, Restricted: doc.IdentityRestricted(),
		}
	}
	if doc.ICB != nil {
		result.IncomingBarring.Active = doc.ICB.Active
	}
	if doc.OCB != nil {
		result.OutgoingBarring.Active = doc.OCB.Active
	}
	return result
}

// UtAccess returns an XCAP client dialed on the XCAP PDN (or the IMS PDN
// when no distinct XCAP APN was configured).
func (i *Instance) UtAccess() (UtAccess, error) {
	if i == nil {
		return UtAccess{}, errors.New("XCAP PDN is not established")
	}
	i.mu.RLock()
	session := i.session
	i.mu.RUnlock()
	if session == nil {
		return UtAccess{}, errors.New("XCAP PDN is not established")
	}
	httpClient := xcap.NewHTTPClient(session.XCAPDialContext())
	if httpClient == nil {
		return UtAccess{}, errors.New("XCAP PDN is not established")
	}
	var xuis []string
	domain := ""
	if session.IMSService != nil {
		xuis = session.IMSService.GetIMPUs()
		status := session.IMSService.StatusCurrent()
		if status != nil {
			domain = strings.TrimSpace(status.Domain)
		}
	}
	xui := ""
	if len(xuis) > 0 {
		xui = strings.TrimSpace(xuis[0])
		if domain == "" {
			domain = domainFromXUI(xui)
		}
	}
	if xui == "" {
		return UtAccess{}, errors.New("IMS public identity is not registered")
	}
	return UtAccess{
		Client: NewUtClient(UtClientConfig{
			HTTP: httpClient, Domain: domain, OnNet: utUsesOnNetHost(session),
		}),
		XUI:  xui,
		More: append([]string(nil), xuis[1:]...),
	}, nil
}

// utUsesOnNetHost is for the IMS PDN, where operator DNS can answer
// xcap.<ims-domain>. A dedicated XCAP APN instead uses the public
// xcap.*.pub.3gppnetwork.org name and dials the RFC1918 A record on
// that PDN (TS 23.003 13.9.1.2).
func utUsesOnNetHost(session *runtimecore.SessionResult) bool {
	return session != nil && session.XCAPNetwork == nil && session.IMSNetwork != nil
}

func domainFromXUI(xui string) string {
	xui = strings.Trim(strings.TrimSpace(xui), "<>")
	_, host, ok := strings.Cut(xui, "@")
	if !ok {
		return ""
	}
	host, _, _ = strings.Cut(host, ";")
	return strings.TrimSpace(host)
}
