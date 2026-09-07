package imscore

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/common"
	"github.com/iniwex5/vowifi-go/internal/vowifi/imsheaders"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

func (s *Service) buildRegistrationSubscription(expires time.Duration) (*sip.Request, time.Duration, error) {
	profile, dialog, err := s.reserveSubscriptionBuildContext()
	if err != nil {
		return nil, 0, fmt.Errorf("imscore: subscription registered profile: %w", err)
	}
	aor, err := parseSubscriptionURI(profile.LocalURI)
	if err != nil {
		return nil, 0, err
	}
	contact, err := buildSubscribeContactHeader(profile.ContactHeader, profile.Transport, true)
	if err != nil {
		return nil, 0, err
	}
	recipient := aor
	if dialog.ready() {
		if target := strings.TrimSpace(dialog.remoteTarget); target != "" {
			if parsed, parseErr := parseSubscriptionURI(target); parseErr == nil {
				recipient = parsed
			}
		}
	}
	options := subscribeRegHeaderOptions(subscribeRegRequestContext{
		profile: profile, aor: aor, contact: contact, expires: expires, dialog: dialog,
	})
	request, err := sipkit.BuildIMSRequest(sip.SUBSCRIBE, recipient, options)
	return request, expires, err
}

type subscribeRegRequestContext struct {
	profile SIPDialogProfile
	aor     sip.Uri
	contact *sip.ContactHeader
	expires time.Duration
	dialog  registrationSubscriptionDialog
}

func subscribeRegHeaderOptions(requestContext subscribeRegRequestContext) sipkit.IMSRequestOptions {
	profile := requestContext.profile
	fromTag := common.RandomHex(10)
	callID := common.RandomHex(20)
	cseq := uint32(profile.InitialCSeq)
	kind := sipkit.RequestKindOutOfDialog
	toTag := ""
	routes := []string(nil)
	if requestContext.dialog.ready() {
		fromTag = requestContext.dialog.localTag
		callID = requestContext.dialog.callID
		cseq = requestContext.dialog.cseq + 1
		kind = sipkit.RequestKindInDialog
		toTag = requestContext.dialog.remoteTag
		routes = append([]string(nil), requestContext.dialog.routeSet...)
	}
	return sipkit.IMSRequestOptions{
		Destination: profile.RemoteAddress, Transport: profile.Transport,
		Branch: "z9hG4bK" + common.RandomHex(36), FromURI: requestContext.aor,
		FromTag: fromTag, ToURI: requestContext.aor, ToTag: toTag,
		CallID: callID, CSeq: cseq, Routes: routes,
		Contact: requestContext.contact, Kind: kind,
		SecurityMode: securityModeIPSec, AddRPort: true, OmitURITransport: true,
		AddUserAgent:      strings.TrimSpace(profile.UserAgent) != "",
		PreferredIdentity: imsheaders.PreferredIdentityHeaderValue(profile.LocalURI),
		Runtime: sipkit.IMSRuntimeSnapshot{
			ServiceRoute: profile.ServiceRoute, SecVerify: profile.SecurityVerify,
			PAccessNetworkInfo: profile.PANI, UserAgent: profile.UserAgent,
			LocalAddr: profile.LocalAddress, Transport: profile.Transport,
		},
		Headers: []sip.Header{
			sip.NewHeader("Expires", strconv.FormatInt(int64(requestContext.expires/time.Second), 10)),
			sip.NewHeader("Event", registrationEventPackage),
			sip.NewHeader("Accept", reginfoContentType),
		},
	}
}

func (s *Service) reserveSubscriptionBuildContext() (SIPDialogProfile, registrationSubscriptionDialog, error) {
	if s == nil || s.cfg == nil {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("service is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.regState != regRegistered || s.regSession == nil {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("registered SIP session is unavailable")
	}
	route := s.registeredSIPRouteLocked()
	if !route.live || route.clientAddress == "" || route.serverAddress == "" {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("registered SIP transport is unavailable")
	}
	if route.securityVerify == "" {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("protected registration security is unavailable")
	}
	localURI := firstNonBlank(s.regSession.publicID, s.reginfoAOR, primaryPublicIdentity(s.cfg))
	registeredContactUser := firstNonBlank(s.regSession.contactUser, contactUser(s.cfg))
	if localURI == "" || registeredContactUser == "" {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("registered subscription identity is unavailable")
	}
	dialog := s.subscriptionDialog
	if !dialog.ready() {
		minimum := s.regSession.cseq + 2
		if s.nextSIPCSeq < minimum {
			s.nextSIPCSeq = minimum
		} else {
			s.nextSIPCSeq++
		}
	}
	contactURI, contactHeader := registeredVoiceContact(s.cfg, registeredContactUser, route.serverAddress)
	return SIPDialogProfile{
		LocalURI: localURI, FromTag: s.regSession.fromTag,
		ContactURI: contactURI, ContactHeader: contactHeader,
		LocalAddress: route.clientAddress, RemoteAddress: route.remoteAddress,
		Transport: route.transport, ServiceRoute: route.serviceRoute,
		SecurityVerify: route.securityVerify, PANI: s.GetPAccessNetworkInfo(),
		UserAgent: strings.TrimSpace(s.cfg.UserAgent), InitialCSeq: s.nextSIPCSeq,
	}, dialog, nil
}

func parseSubscriptionURI(value string) (sip.Uri, error) {
	var uri sip.Uri
	if err := sip.ParseUri(strings.TrimSpace(value), &uri); err != nil {
		return sip.Uri{}, fmt.Errorf("imscore: subscription AOR: %w", err)
	}
	return uri, nil
}

func buildSubscribeContactHeader(value, transport string, protected bool) (*sip.ContactHeader, error) {
	var uri sip.Uri
	params := sip.NewParams()
	displayName, err := sip.ParseAddressValue(strings.TrimSpace(value), &uri, &params)
	if err != nil {
		return nil, fmt.Errorf("imscore: subscription Contact: %w", err)
	}
	if protected {
		transport = "tcp"
	}
	if transport = strings.ToLower(strings.TrimSpace(transport)); transport != "" {
		uri.UriParams.Add("transport", transport)
	}
	return &sip.ContactHeader{DisplayName: displayName, Address: uri, Params: params}, nil
}

func (s *Service) buildMWISubscription(expires time.Duration) (*sip.Request, time.Duration, error) {
	profile, dialog, err := s.reserveMWISubscriptionBuildContext()
	if err != nil {
		return nil, 0, fmt.Errorf("imscore: MWI subscription registered profile: %w", err)
	}
	aor, err := parseSubscriptionURI(profile.LocalURI)
	if err != nil {
		return nil, 0, err
	}
	contact, err := buildSubscribeContactHeader(profile.ContactHeader, profile.Transport, true)
	if err != nil {
		return nil, 0, err
	}
	recipient := aor
	if dialog.ready() {
		if target := strings.TrimSpace(dialog.remoteTarget); target != "" {
			if parsed, parseErr := parseSubscriptionURI(target); parseErr == nil {
				recipient = parsed
			}
		}
	}
	options := subscribeMWIHeaderOptions(subscribeRegRequestContext{
		profile: profile, aor: aor, contact: contact, expires: expires, dialog: dialog,
	})
	request, err := sipkit.BuildIMSRequest(sip.SUBSCRIBE, recipient, options)
	return request, expires, err
}

func subscribeMWIHeaderOptions(requestContext subscribeRegRequestContext) sipkit.IMSRequestOptions {
	options := subscribeRegHeaderOptions(requestContext)
	options.Headers = []sip.Header{
		sip.NewHeader("Expires", strconv.FormatInt(int64(requestContext.expires/time.Second), 10)),
		sip.NewHeader("Event", mwiEventPackage),
		sip.NewHeader("Accept", mwiContentType),
	}
	return options
}

func (s *Service) reserveMWISubscriptionBuildContext() (SIPDialogProfile, registrationSubscriptionDialog, error) {
	if s == nil || s.cfg == nil {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("service is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.regState != regRegistered || s.regSession == nil {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("registered SIP session is unavailable")
	}
	route := s.registeredSIPRouteLocked()
	if !route.live || route.clientAddress == "" || route.serverAddress == "" {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("registered SIP transport is unavailable")
	}
	if route.securityVerify == "" {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("protected registration security is unavailable")
	}
	localURI := firstNonBlank(s.regSession.publicID, s.reginfoAOR, primaryPublicIdentity(s.cfg))
	registeredContactUser := firstNonBlank(s.regSession.contactUser, contactUser(s.cfg))
	if localURI == "" || registeredContactUser == "" {
		return SIPDialogProfile{}, registrationSubscriptionDialog{}, errors.New("registered subscription identity is unavailable")
	}
	dialog := s.mwiSubscriptionDialog
	if !dialog.ready() {
		minimum := s.regSession.cseq + 2
		if s.nextSIPCSeq < minimum {
			s.nextSIPCSeq = minimum
		} else {
			s.nextSIPCSeq++
		}
	}
	contactURI, contactHeader := registeredVoiceContact(s.cfg, registeredContactUser, route.serverAddress)
	return SIPDialogProfile{
		LocalURI: localURI, FromTag: s.regSession.fromTag,
		ContactURI: contactURI, ContactHeader: contactHeader,
		LocalAddress: route.clientAddress, RemoteAddress: route.remoteAddress,
		Transport: route.transport, ServiceRoute: route.serviceRoute,
		SecurityVerify: route.securityVerify, PANI: s.GetPAccessNetworkInfo(),
		UserAgent: strings.TrimSpace(s.cfg.UserAgent), InitialCSeq: s.nextSIPCSeq,
	}, dialog, nil
}
