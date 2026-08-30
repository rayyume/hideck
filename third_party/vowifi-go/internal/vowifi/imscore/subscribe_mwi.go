package imscore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/events"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

const (
	mwiEventPackage        = "message-summary"
	mwiContentType         = "application/simple-message-summary"
	mwiSubscriptionTimeout = 10 * time.Second
	mwiSubscriptionFlow    = "subscribe_mwi"
)

func (s *Service) startMWISubscription() {
	s.resetMWISubscription()
	eligible, skipReason := s.registrationSubscriptionGate()
	if !eligible {
		logging.Info("IMS SUBSCRIBE(mwi) skipped",
			"device", s.DeviceID(), "reason", skipReason)
		return
	}
	logging.Info("IMS SUBSCRIBE(mwi) starting", "device", s.DeviceID())
	s.networkDone.Add(1)
	go func() {
		defer s.networkDone.Done()
		ctx, cancel := context.WithTimeout(context.Background(), mwiSubscriptionTimeout)
		defer cancel()
		if err := s.sendSubscribeMWI(ctx); err != nil {
			s.reportMWISubscriptionRuntimeError(err)
		}
	}()
}

func (s *Service) resetMWISubscription() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.mwiSubscriptionClosed = false
	s.mwiSubscriptionDialog = registrationSubscriptionDialog{}
	s.mwiSubscriptionRefreshAt = time.Time{}
	s.mwiSubscriptionExpires = 0
	s.mwiSubscriptionLastErr = ""
	s.mu.Unlock()
}

func (s *Service) mwiSubscriptionEligibleLocked() bool {
	eligible, _ := s.subscriptionGateLocked()
	return eligible && !s.mwiSubscriptionClosed
}

func (s *Service) reportMWISubscriptionRuntimeError(err error) {
	if err == nil || s.stopped() {
		return
	}
	if !s.hasProtectedRegistrationTransport() {
		logging.Debug("IMS SUBSCRIBE(mwi) result discarded after registration changed",
			"device", s.DeviceID(), "err", err)
		return
	}
	logging.WarnRate("ims-subscribe-mwi-"+s.DeviceID(), 30*time.Second,
		"IMS SUBSCRIBE(mwi) failed; keeping current registration",
		"device", s.DeviceID(), "err", err)
}

func (s *Service) sendSubscribeMWI(ctx context.Context) error {
	return s.sendMWISubscription(ctx, registerExpires(s.cfg), false)
}

func (s *Service) sendUnsubscribeMWI(ctx context.Context) error {
	return s.sendMWISubscription(ctx, 0, true)
}

func (s *Service) unsubscribeMWI(ctx context.Context) {
	if s == nil || !s.hasMWISubscriptionDialog() {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	unsubCtx, cancel := context.WithTimeout(ctx, mwiSubscriptionTimeout)
	defer cancel()
	if err := s.sendUnsubscribeMWI(unsubCtx); err != nil {
		logging.Info("IMS SUBSCRIBE(mwi) unsubscribe failed",
			"device", s.DeviceID(), "err", err)
	}
}

func (s *Service) hasMWISubscriptionDialog() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mwiSubscriptionDialog.ready()
}

func (s *Service) sendMWISubscription(ctx context.Context, expires time.Duration, unsubscribe bool) error {
	if unsubscribe && !s.hasMWISubscriptionDialog() {
		return nil
	}
	if !s.mwiSubscriptionInFlight.CompareAndSwap(false, true) {
		if unsubscribe {
			return errors.New("imscore: MWI subscription is already in flight")
		}
		return nil
	}
	defer s.mwiSubscriptionInFlight.Store(false)
	s.subscribeMu.Lock()
	defer s.subscribeMu.Unlock()
	if unsubscribe && !s.hasMWISubscriptionDialog() {
		return nil
	}

	request, requestedExpires, err := s.buildMWISubscription(expires)
	if err != nil {
		return s.recordMWISubscriptionResult(nil, 0, unsubscribe, err)
	}
	response, err := s.exchangeMWISubscription(ctx, request, requestedExpires)
	if err != nil {
		return s.recordMWISubscriptionResult(nil, requestedExpires, unsubscribe, err)
	}
	if response.StatusCode == 481 && !unsubscribe && s.hasMWISubscriptionDialog() {
		s.clearMWISubscriptionDialog()
		logging.Info("IMS SUBSCRIBE(mwi) dialog gone; retrying as initial",
			"device", s.DeviceID())
		request, requestedExpires, err = s.buildMWISubscription(expires)
		if err != nil {
			return s.recordMWISubscriptionResult(nil, 0, false, err)
		}
		response, err = s.exchangeMWISubscription(ctx, request, requestedExpires)
		if err != nil {
			return s.recordMWISubscriptionResult(nil, requestedExpires, false, err)
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err = fmt.Errorf("SUBSCRIBE rejected with status %d (%s)", response.StatusCode, response.Reason)
		return s.recordMWISubscriptionResult(response, requestedExpires, unsubscribe, err)
	}
	s.learnMWISubscriptionDialog(request, response)
	if err := s.recordMWISubscriptionResult(response, requestedExpires, unsubscribe, nil); err != nil {
		return err
	}
	if unsubscribe {
		logging.Info("IMS SUBSCRIBE(mwi) unsubscribed", "call_id", request.CallID().Value())
		return nil
	}
	logging.Info("IMS SUBSCRIBE(mwi) succeeded", "call_id", request.CallID().Value(), "code", response.StatusCode)
	return nil
}

func (s *Service) exchangeMWISubscription(
	ctx context.Context,
	request *sip.Request,
	requestedExpires time.Duration,
) (*sip.Response, error) {
	s.recordMWISubscriptionAttempt(time.Now(), requestedExpires)
	logging.Debug("IMS SUBSCRIBE(mwi) outbound", "device", s.DeviceID(), "sip", logging.RedactSIPRaw(request.String()))
	response, _, err := s.dispatchOutboundRequest(
		ctx, mwiSubscriptionFlow, request, mwiSubscriptionTimeout, true,
	)
	if err != nil {
		return nil, fmt.Errorf("SUBSCRIBE transaction: %w", err)
	}
	return response, nil
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

func (s *Service) recordMWISubscriptionAttempt(at time.Time, expires time.Duration) {
	s.mu.Lock()
	s.mwiSubscriptionLastAttemptAt = at
	s.mwiSubscriptionExpires = expires
	s.mwiSubscriptionRefreshAt = at.Add(subscriptionRefreshDelay(expires))
	s.mwiSubscriptionLastErr = ""
	s.mu.Unlock()
	s.signalIMSMaintenance()
}

func (s *Service) recordMWISubscriptionResult(
	response *sip.Response,
	requestedExpires time.Duration,
	unsubscribe bool,
	resultErr error,
) error {
	completedAt := time.Now()
	s.mu.Lock()
	if resultErr != nil {
		s.mwiSubscriptionLastErr = resultErr.Error()
		if subscriptionPermanentlyRejected(response) {
			s.mwiSubscriptionClosed = true
			s.mwiSubscriptionDialog = registrationSubscriptionDialog{}
			s.mwiSubscriptionExpires = 0
			s.mwiSubscriptionRefreshAt = time.Time{}
		}
		s.mu.Unlock()
		return resultErr
	}
	s.mwiSubscriptionLastOKAt = completedAt
	s.mwiSubscriptionLastErr = ""
	if unsubscribe || requestedExpires <= 0 {
		s.mwiSubscriptionClosed = true
		s.mwiSubscriptionDialog = registrationSubscriptionDialog{}
		s.mwiSubscriptionExpires = 0
		s.mwiSubscriptionRefreshAt = time.Time{}
		s.mu.Unlock()
		return nil
	}
	expires := subscriptionExpires(response, requestedExpires)
	s.mwiSubscriptionExpires = expires
	s.mwiSubscriptionRefreshAt = completedAt.Add(subscriptionRefreshDelay(expires))
	s.mu.Unlock()
	s.signalIMSMaintenance()
	return nil
}

func (s *Service) learnMWISubscriptionDialog(request *sip.Request, response *sip.Response) {
	if s == nil || request == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dialog := s.mwiSubscriptionDialog
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
	s.mwiSubscriptionDialog = dialog
}

func (s *Service) clearMWISubscriptionDialog() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.mwiSubscriptionDialog = registrationSubscriptionDialog{}
	s.mu.Unlock()
}

func (s *Service) closeMWISubscription() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.mwiSubscriptionClosed = true
	s.mwiSubscriptionDialog = registrationSubscriptionDialog{}
	s.mwiSubscriptionRefreshAt = time.Time{}
	s.mwiSubscriptionExpires = 0
	s.mu.Unlock()
}

func (s *Service) learnMWISubscriptionDialogFromNotify(raw string) {
	if s == nil || !isMWINotification(raw) {
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
	dialog := s.mwiSubscriptionDialog
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
	s.mwiSubscriptionDialog = dialog
}

func (s *Service) refreshMWISubscription() {
	ctx, cancel := context.WithTimeout(context.Background(), mwiSubscriptionTimeout)
	defer cancel()
	if err := s.sendSubscribeMWI(ctx); err != nil {
		s.reportMWISubscriptionRuntimeError(err)
	}
}

func sipEventPackage(raw string) string {
	event, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(rawSIPHeaderValue(raw, "Event"))), ";")
	return strings.TrimSpace(event)
}

func (s *Service) handleInboundNotification(raw string) {
	switch sipEventPackage(raw) {
	case mwiEventPackage:
		s.handleMWINotification(raw)
	case registrationEventPackage:
		s.handleRegistrationNotification(raw)
	default:
		logging.Info("IMS NOTIFY acknowledged", "event", rawSIPHeaderValue(raw, "Event"))
	}
}

func isMWINotification(raw string) bool {
	if sipEventPackage(raw) != mwiEventPackage {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(rawSIPHeaderValue(raw, "Content-Type")))
	return contentType == "" || strings.Contains(contentType, "simple-message-summary")
}

func (s *Service) handleMWINotification(raw string) {
	logging.Info("IMS NOTIFY acknowledged", "event", mwiEventPackage)
	if !isMWINotification(raw) {
		return
	}
	s.learnMWISubscriptionDialogFromNotify(raw)
	if subscriptionStateTerminated(raw) {
		s.closeMWISubscription()
	}
	body, err := rawSIPBody(raw)
	if err != nil {
		logging.WarnRate("ims-mwi-body", "IMS MWI body is invalid", "err", err)
		return
	}
	summary := parseMWISummary(string(body))
	s.mu.Lock()
	s.mwiLastSummary = summary.raw
	s.mwiMessagesWaiting = summary.waiting
	deviceID := ""
	if s.cfg != nil {
		deviceID = s.cfg.DeviceID
	}
	s.mu.Unlock()
	logging.Info("IMS MWI updated",
		"device", deviceID,
		"waiting", summary.waiting,
		"voice_new", summary.voiceNew,
		"voice_old", summary.voiceOld)
	s.publishRuntimeEvent(events.EventMWIUpdated{
		DevID:           deviceID,
		MessagesWaiting: summary.waiting,
		VoiceNew:        summary.voiceNew,
		VoiceOld:        summary.voiceOld,
		Account:         summary.account,
		Time:            time.Now(),
	})
}
