package imscore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
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
	remoteCSeq   uint32
	notifySeen   bool
}

func (d registrationSubscriptionDialog) ready() bool {
	return strings.TrimSpace(d.callID) != "" &&
		strings.TrimSpace(d.localTag) != "" &&
		strings.TrimSpace(d.remoteTag) != ""
}

func (s *Service) startRegistrationSubscription() {
	eligible, skipReason := s.prepareSubscriptionStart(false)
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

func (s *Service) resetRegistrationSubscriptionLocked() {
	s.subscriptionClosed = false
	s.subscriptionDialog = registrationSubscriptionDialog{}
	s.subscriptionRefreshAt = time.Time{}
	s.subscriptionExpires = 0
	s.subscriptionLastErr = ""
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
	if errors.Is(err, errSubscriptionContextChanged) {
		logging.Debug("IMS SUBSCRIBE(reg) retired attempt completed; checking current registration", "device", s.DeviceID(), "err", err)
		s.startRegistrationSubscription()
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

	attempt, err := s.beginSubscriptionAttempt(false, unsubscribe)
	if err != nil {
		return err
	}
	result := subscriptionResult{context: attempt, unsubscribe: unsubscribe}
	request, requestedExpires, err := s.buildRegistrationSubscription(expires)
	result.request, result.requestedExpires, result.err = request, requestedExpires, err
	if err != nil {
		return s.recordSubscriptionResult(result)
	}
	response, err := s.exchangeRegistrationSubscription(ctx, result)
	result.response, result.err = response, err
	if err != nil {
		return s.recordSubscriptionResult(result)
	}
	if response.StatusCode == 481 && !unsubscribe && s.retrySubscriptionAfter481(result, false) {
		logging.Info("IMS SUBSCRIBE(reg) dialog gone; retrying as initial",
			"device", s.DeviceID())
		request, requestedExpires, err = s.buildRegistrationSubscription(expires)
		result.request, result.requestedExpires, result.err = request, requestedExpires, err
		if err != nil {
			return s.recordSubscriptionResult(result)
		}
		response, err = s.exchangeRegistrationSubscription(ctx, result)
		result.response, result.err = response, err
		if err != nil {
			return s.recordSubscriptionResult(result)
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.err = fmt.Errorf("SUBSCRIBE rejected with status %d (%s)", response.StatusCode, response.Reason)
		return s.recordSubscriptionResult(result)
	}
	if err := s.recordSubscriptionResult(result); err != nil {
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
	result subscriptionResult,
) (*sip.Response, error) {
	if err := s.recordSubscriptionAttempt(result); err != nil {
		return nil, err
	}
	request := result.request
	logging.Debug("IMS SUBSCRIBE(reg) outbound", "device", s.DeviceID(), "sip", logging.RedactSIPRaw(request.String()))
	response, _, err := s.dispatchOutboundRequestWithCallbacks(outboundDispatchOptions{
		Context: ctx, Flow: registrationSubscriptionFlow, Request: request, Timeout: registrationSubscriptionTimeout,
		Callbacks: sipTransactionCallbacks{onBeforeSend: func() error { return s.subscriptionSent(result, false) }},
	}, true)
	if err != nil {
		return nil, fmt.Errorf("SUBSCRIBE transaction: %w", err)
	}
	return response, nil
}

func (s *Service) recordSubscriptionAttempt(result subscriptionResult) error {
	return s.recordSubscriptionUsageAttempt(result, false)
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

func (s *Service) recordSubscriptionResult(result subscriptionResult) error {
	return s.recordSubscriptionUsageResult(result, false)
}

func (s *Service) learnSubscriptionDialogLocked(request *sip.Request, response *sip.Response) {
	s.subscriptionDialog = learnSubscriptionDialog(s.subscriptionDialog, request, response)
}
