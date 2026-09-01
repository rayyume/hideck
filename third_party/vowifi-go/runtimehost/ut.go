package runtimehost

import (
	"errors"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/runtimecore"
	"github.com/iniwex5/vowifi-go/xcap"
)

// UtAccess is the XCAP HTTP client and public identities for one device.
type UtAccess struct {
	Client *xcap.Client
	XUI    string
	More   []string
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
	client := xcap.NewHTTPClient(session.XCAPDialContext())
	if client == nil {
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
		Client: &xcap.Client{
			HTTP: client, Domain: domain,
			OnNet: utUsesOnNetHost(session),
		},
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
