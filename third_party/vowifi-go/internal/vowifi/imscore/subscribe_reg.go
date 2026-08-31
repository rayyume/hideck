package imscore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/common"
	"github.com/iniwex5/vowifi-go/internal/vowifi/imsheaders"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

const (
	registrationSubscriptionTimeout = 10 * time.Second
	registrationSubscriptionFlow    = "subscribe_reg"
)

type registrationSubscriptionDialog struct {
	callID       string
	localTag     string
	remoteTag    string
	remoteTarget string
	routeSet     []string
	cseq         uint32
}

func (d registrationSubscriptionDialog) ready() bool {
	return strings.TrimSpace(d.callID) != "" &&
		strings.TrimSpace(d.localTag) != "" &&
		strings.TrimSpace(d.remoteTag) != ""
}

func (s *Service) startRegistrationSubscription() {
	s.resetRegistrationSubscription()
	eligible, skipReason := s.registrationSubscriptionGate()
	if !eligible {
		logging.Info("IMS SUBSCRIBE(reg) skipped",
			"device", s.DeviceID(), "reason", skipReason)
		return
	}
	logging.Info("IMS SUBSCRIBE(reg) starting", "device", s.DeviceID())
	s.networkDone.Add(1)
	go func() {
		defer s.networkDone.Done()
		ctx, cancel := context.WithTimeout(context.Background(), registrationSubscriptionTimeout)
		defer cancel()
		if err := s.sendSubscribeReg(ctx); err != nil {
			s.reportSubscriptionRuntimeError(err)
		}
	}()
}

func (s *Service) resetRegistrationSubscription() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.subscriptionClosed = false
	s.subscriptionDialog = registrationSubscriptionDialog{}
	s.subscriptionRefreshAt = time.Time{}
	s.subscriptionExpires = 0
	s.subscriptionLastErr = ""
	s.mu.Unlock()
}

func (s *Service) hasProtectedRegistrationTransport() bool {
	eligible, _ := s.registrationSubscriptionGate()
	return eligible
}

func (s *Service) registrationSubscriptionGate() (bool, string) {
	if s == nil {
		return false, "service_nil"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.subscriptionGateLocked()
}

func (s *Service) subscriptionGateLocked() (bool, string) {
	if s.regState != regRegistered {
		return false, "not_registered:" + strings.TrimSpace(s.regState)
	}
	if s.registrationTCP == nil {
		return false, "no_registration_tcp"
	}
	if s.regSession == nil {
		return false, "no_reg_session"
	}
	if s.regSession.security == nil || strings.TrimSpace(s.regSession.security.verifyHeader) == "" {
		return false, "no_sec_agree"
	}
	return true, ""
}

func (s *Service) subscriptionEligibleLocked() bool {
	eligible, _ := s.subscriptionGateLocked()
	return eligible && !s.subscriptionClosed
}

func (s *Service) stopped() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
}

func (s *Service) reportRegistrationRuntimeError(err error) {
	if err == nil || s == nil || s.stopped() {
		return
	}
	logging.Info("IMS runtime reconnect requested", "device", s.DeviceID(), "err", err)
	select {
	case s.registerErrors <- err:
	default:
		logging.WarnRate("ims-runtime-error-overflow-"+s.DeviceID(), time.Minute,
			"IMS runtime error channel is full", "device", s.DeviceID(), "err", err)
	}
}

func (s *Service) reportSubscriptionRuntimeError(err error) {
	if err == nil || s.stopped() {
		return
	}
	if !s.hasProtectedRegistrationTransport() {
		logging.Debug("IMS SUBSCRIBE result discarded after registration changed",
			"device", s.DeviceID(), "err", err)
		return
	}
	// SUBSCRIBE(reg) is for network-initiated deregister NOTIFY. A reject or
	// timeout must not tear down a REGISTER that already succeeded — that
	// produced a one-second IMS-ready flash then a full SWu rebuild.
	logging.WarnRate("ims-subscribe-reg-"+s.DeviceID(), 30*time.Second,
		"IMS SUBSCRIBE(reg) failed; keeping current registration",
		"device", s.DeviceID(), "err", subscriptionRuntimeError(err))
}

func (s *Service) sendSubscribeReg(ctx context.Context) error {
	return s.sendRegistrationSubscription(ctx, registerExpires(s.cfg), false)
}

func (s *Service) sendUnsubscribeReg(ctx context.Context) error {
	return s.sendRegistrationSubscription(ctx, 0, true)
}

func (s *Service) unsubscribeRegistration(ctx context.Context) {
	if s == nil || !s.hasSubscriptionDialog() {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	unsubCtx, cancel := context.WithTimeout(ctx, registrationSubscriptionTimeout)
	defer cancel()
	if err := s.sendUnsubscribeReg(unsubCtx); err != nil {
		logging.Info("IMS SUBSCRIBE(reg) unsubscribe failed",
			"device", s.DeviceID(), "err", err)
	}
}

func (s *Service) hasSubscriptionDialog() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.subscriptionDialog.ready()
}

func (s *Service) sendRegistrationSubscription(ctx context.Context, expires time.Duration, unsubscribe bool) error {
	if unsubscribe && !s.hasSubscriptionDialog() {
		return nil
	}
	if !s.subscriptionInFlight.CompareAndSwap(false, true) {
		if unsubscribe {
			return errors.New("imscore: registration subscription is already in flight")
		}
		return nil
	}
	defer s.subscriptionInFlight.Store(false)
	s.subscribeMu.Lock()
	defer s.subscribeMu.Unlock()
	if unsubscribe && !s.hasSubscriptionDialog() {
		return nil
	}

	request, requestedExpires, err := s.buildRegistrationSubscription(expires)
	if err != nil {
		return s.recordSubscriptionResult(nil, 0, unsubscribe, err)
	}
	response, err := s.exchangeRegistrationSubscription(ctx, request, requestedExpires)
	if err != nil {
		return s.recordSubscriptionResult(nil, requestedExpires, unsubscribe, err)
	}
	if response.StatusCode == 481 && !unsubscribe && s.hasSubscriptionDialog() {
		s.clearSubscriptionDialog()
		logging.Info("IMS SUBSCRIBE(reg) dialog gone; retrying as initial",
			"device", s.DeviceID())
		request, requestedExpires, err = s.buildRegistrationSubscription(expires)
		if err != nil {
			return s.recordSubscriptionResult(nil, 0, false, err)
		}
		response, err = s.exchangeRegistrationSubscription(ctx, request, requestedExpires)
		if err != nil {
			return s.recordSubscriptionResult(nil, requestedExpires, false, err)
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err = fmt.Errorf("SUBSCRIBE rejected with status %d (%s)", response.StatusCode, response.Reason)
		return s.recordSubscriptionResult(response, requestedExpires, unsubscribe, err)
	}
	s.learnSubscriptionDialog(request, response)
	if err := s.recordSubscriptionResult(response, requestedExpires, unsubscribe, nil); err != nil {
		return err
	}
	if unsubscribe {
		logging.Info("IMS SUBSCRIBE(reg) unsubscribed", "call_id", request.CallID().Value())
		return nil
	}
	logging.Info("IMS SUBSCRIBE(reg) succeeded", "call_id", request.CallID().Value(), "code", response.StatusCode)
	return nil
}

func (s *Service) exchangeRegistrationSubscription(
	ctx context.Context,
	request *sip.Request,
	requestedExpires time.Duration,
) (*sip.Response, error) {
	s.recordSubscriptionAttempt(time.Now(), requestedExpires)
	logging.Debug("IMS SUBSCRIBE(reg) outbound", "device", s.DeviceID(), "sip", logging.RedactSIPRaw(request.String()))
	response, _, err := s.dispatchOutboundRequest(
		ctx, registrationSubscriptionFlow, request, registrationSubscriptionTimeout, true,
	)
	if err != nil {
		return nil, fmt.Errorf("SUBSCRIBE transaction: %w", err)
	}
	return response, nil
}

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

func (s *Service) recordSubscriptionAttempt(at time.Time, expires time.Duration) {
	s.mu.Lock()
	s.subscriptionLastAttemptAt = at
	s.subscriptionExpires = expires
	s.subscriptionRefreshAt = at.Add(subscriptionRefreshDelay(expires))
	s.subscriptionLastErr = ""
	s.mu.Unlock()
	s.signalIMSMaintenance()
}

func subscriptionPermanentlyRejected(response *sip.Response) bool {
	if response == nil {
		return false
	}
	switch response.StatusCode {
	case 403, 405, 489:
		return true
	default:
		return false
	}
}

func (s *Service) recordSubscriptionResult(
	response *sip.Response,
	requestedExpires time.Duration,
	unsubscribe bool,
	resultErr error,
) error {
	completedAt := time.Now()
	s.mu.Lock()
	if resultErr != nil {
		s.subscriptionLastErr = resultErr.Error()
		if subscriptionPermanentlyRejected(response) {
			s.subscriptionClosed = true
			s.subscriptionDialog = registrationSubscriptionDialog{}
			s.subscriptionExpires = 0
			s.subscriptionRefreshAt = time.Time{}
		}
		s.mu.Unlock()
		return resultErr
	}
	s.subscriptionLastOKAt = completedAt
	s.subscriptionLastErr = ""
	if unsubscribe || requestedExpires <= 0 {
		s.subscriptionClosed = true
		s.subscriptionDialog = registrationSubscriptionDialog{}
		s.subscriptionExpires = 0
		s.subscriptionRefreshAt = time.Time{}
		s.mu.Unlock()
		return nil
	}
	expires := subscriptionExpires(response, requestedExpires)
	s.subscriptionExpires = expires
	s.subscriptionRefreshAt = completedAt.Add(subscriptionRefreshDelay(expires))
	s.mu.Unlock()
	s.signalIMSMaintenance()
	return nil
}

func (s *Service) learnSubscriptionDialog(request *sip.Request, response *sip.Response) {
	if s == nil || request == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dialog := s.subscriptionDialog
	if request.CallID() != nil {
		dialog.callID = request.CallID().Value()
	}
	if tag := fromHeaderTag(request.From()); tag != "" {
		dialog.localTag = tag
	}
	if request.CSeq() != nil {
		dialog.cseq = request.CSeq().SeqNo
	}
	if response != nil {
		if tag := toHeaderTag(response.To()); tag != "" {
			dialog.remoteTag = tag
		}
		if target := firstSIPHeaderURI(sipkit.FirstHeaderValue(response, "Contact", true)); target != "" {
			dialog.remoteTarget = target
		}
		if routes := recordRouteSet(response); len(routes) > 0 {
			dialog.routeSet = routes
		}
	}
	s.subscriptionDialog = dialog
}

func (s *Service) clearSubscriptionDialog() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.subscriptionDialog = registrationSubscriptionDialog{}
	s.mu.Unlock()
}

func (s *Service) closeRegistrationSubscription() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.subscriptionClosed = true
	s.subscriptionDialog = registrationSubscriptionDialog{}
	s.subscriptionRefreshAt = time.Time{}
	s.subscriptionExpires = 0
	s.mu.Unlock()
}

func (s *Service) learnSubscriptionDialogFromNotify(raw string) {
	if s == nil || !isRegistrationNotification(raw) {
		return
	}
	callID := strings.TrimSpace(rawSIPHeaderValue(raw, "Call-ID"))
	remoteTag := sipAddressTag(rawSIPHeaderValue(raw, "From"))
	localTag := sipAddressTag(rawSIPHeaderValue(raw, "To"))
	if callID == "" || remoteTag == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dialog := s.subscriptionDialog
	if dialog.callID != "" && !strings.EqualFold(dialog.callID, callID) {
		return
	}
	dialog.callID = callID
	if localTag != "" {
		dialog.localTag = localTag
	}
	dialog.remoteTag = remoteTag
	if target := firstSIPHeaderURI(rawSIPHeaderValue(raw, "Contact")); target != "" {
		dialog.remoteTarget = target
	}
	s.subscriptionDialog = dialog
}

func recordRouteSet(message sip.Message) []string {
	if message == nil {
		return nil
	}
	headers := message.GetHeaders("Record-Route")
	routes := make([]string, 0, len(headers))
	for _, header := range headers {
		if value := strings.TrimSpace(header.Value()); value != "" {
			routes = append(routes, value)
		}
	}
	for i, j := 0, len(routes)-1; i < j; i, j = i+1, j-1 {
		routes[i], routes[j] = routes[j], routes[i]
	}
	return routes
}

func sipAddressTag(value string) string {
	for _, part := range strings.Split(value, ";") {
		name, tag, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(name, "tag") {
			return strings.TrimSpace(tag)
		}
	}
	return ""
}

func subscriptionStateTerminated(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(rawSIPHeaderValue(raw, "Subscription-State")))
	state, _, _ := strings.Cut(value, ";")
	return strings.TrimSpace(state) == "terminated"
}

func subscriptionRefreshDelay(expires time.Duration) time.Duration {
	if expires > imsSubscriptionRefreshAdvance {
		return expires - imsSubscriptionRefreshAdvance
	}
	return 0
}

func subscriptionExpires(response *sip.Response, fallback time.Duration) time.Duration {
	if response == nil {
		return fallback
	}
	value := sipkit.FirstHeaderValue(response, "Expires", true)
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func subscriptionRuntimeError(err error) error {
	return fmt.Errorf("imscore: registration event subscription failed: %w", err)
}

func firstSIPHeaderURI(value string) string {
	value = strings.TrimSpace(value)
	if start := strings.IndexByte(value, '<'); start >= 0 {
		if end := strings.IndexByte(value[start+1:], '>'); end >= 0 {
			return strings.TrimSpace(value[start+1 : start+1+end])
		}
	}
	value, _, _ = strings.Cut(value, ",")
	return strings.Trim(strings.TrimSpace(value), "<>")
}
