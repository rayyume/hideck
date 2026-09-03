package imscore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const giffgaffCarrierPresetID = "giffgaff_23410"

var registrationCleanupAttempts sync.Map

// ClearRegistrationBindings removes every registrar binding for the current
// public identity. The caller must immediately register again on success.
func (s *Service) ClearRegistrationBindings(ctx context.Context) error {
	if s == nil {
		return errors.New("imscore: nil service")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	return s.clearRegistrationBindingsLocked(ctx)
}

func (s *Service) clearRegistrationBindingsLocked(ctx context.Context) error {
	session, err := s.registrationBindingCleanupSession()
	if err != nil {
		return err
	}
	response, err := s.exchangeWildcardUnregister(ctx, session)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("imscore: wildcard deregistration rejected with status %d %s",
			response.StatusCode, strings.TrimSpace(response.Reason))
	}
	s.mu.Lock()
	s.regSession = session
	s.mu.Unlock()
	logging.Info("IMS registrar bindings cleared", "device", s.DeviceID(), "cseq", session.cseq)
	return nil
}

func (s *Service) requestRegistrationBindingCleanup(document *regInfoDocument) bool {
	if s == nil || s.cfg == nil {
		return false
	}
	s.mu.RLock()
	contactID, contactNeedle := s.registrationContactIdentityLocked()
	cleanupKey := s.registrationBindingCleanupKeyLocked()
	s.mu.RUnlock()
	if cleanupKey == "" || !hasDuplicateActiveRegistration(document, contactID, contactNeedle) {
		return false
	}
	if _, attempted := registrationCleanupAttempts.LoadOrStore(cleanupKey, struct{}{}); attempted {
		return false
	}
	s.bindingCleanupPending.Store(true)
	logging.Info("IMS duplicate registration bindings require cleanup", "device", s.DeviceID())
	return true
}

func (s *Service) registrationBindingCleanupKeyLocked() string {
	if s.regSession == nil {
		return ""
	}
	publicID := strings.TrimSpace(s.regSession.publicID)
	if publicID == "" && s.cfg != nil {
		publicID = strings.TrimSpace(s.cfg.IMPU)
	}
	deviceID := strings.TrimSpace(s.DeviceID())
	if deviceID == "" || publicID == "" || strings.TrimSpace(s.regSession.authHeader) == "" {
		return ""
	}
	return deviceID + "\x00" + publicID
}

func hasDuplicateActiveRegistration(
	document *regInfoDocument,
	contactID string,
	contactNeedle string,
) bool {
	if document == nil || (contactID == "" && contactNeedle == "") {
		return false
	}
	for _, registration := range document.Registrations {
		activeCount := 0
		currentActive := false
		for _, contact := range registration.Contacts {
			state := strings.ToLower(strings.TrimSpace(contact.State))
			if state != "active" && state != "registered" {
				continue
			}
			activeCount++
			currentActive = currentActive || reginfoContactMatches(contact, contactID, contactNeedle)
		}
		if currentActive && activeCount > 1 {
			return true
		}
	}
	return false
}

func (s *Service) registrationBindingCleanupSession() (*registerSession, error) {
	session, err := s.nextRegisterSession("binding cleanup")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(session.authHeader) == "" {
		return nil, errors.New("imscore: registered session has no authorization for binding cleanup")
	}
	return session, nil
}

func (s *Service) nextRegisterSession(operation string) (*registerSession, error) {
	s.mu.RLock()
	current := s.regSession
	if current == nil {
		s.mu.RUnlock()
		return nil, fmt.Errorf("imscore: no registered session for %s", operation)
	}
	session := &registerSession{
		callID: current.callID, fromTag: current.fromTag,
		contactUser: current.contactUser, cseq: current.cseq + 1,
		challenge: current.challenge, authHeader: current.authHeader,
		expires: current.expires, security: current.security,
		publicID: current.publicID, serviceRoute: current.serviceRoute,
		path: current.path, template: current.template,
	}
	s.mu.RUnlock()
	return session, nil
}

func (s *Service) exchangeWildcardUnregister(
	ctx context.Context,
	session *registerSession,
) (*sipResponse, error) {
	return s.exchangeUnregister(ctx, session, true)
}

func (s *Service) exchangeUnregister(
	ctx context.Context,
	session *registerSession,
	wildcard bool,
) (*sipResponse, error) {
	s.recordRegisterSession(session)
	request := s.buildContactUnregister(session, session.authHeader)
	kind := "contact"
	if wildcard {
		request = s.buildWildcardUnregister(session, session.authHeader)
		kind = "wildcard"
	}
	logging.Info("IMS deregistration outbound", "device", s.DeviceID(), "kind", kind, "cseq", session.cseq)
	logging.RunDebug("IMS deregistration outbound", "kind", kind, "cseq", session.cseq,
		"sip", logging.RedactSIPRaw(request))
	response, err := s.transport.RoundTrip(ctx, request)
	if err != nil {
		logging.Info("IMS deregistration transaction failed",
			"device", s.DeviceID(), "kind", kind, "cseq", session.cseq, "err", err)
		return nil, fmt.Errorf("imscore: %s deregistration CSeq %d transaction: %w",
			kind, session.cseq, err)
	}
	logging.Info("IMS deregistration response", "device", s.DeviceID(), "kind", kind,
		"cseq", session.cseq, "status", response.StatusCode)
	matched, matchErr := matchesRegisterTransaction(response, session)
	if matchErr != nil {
		return nil, matchErr
	}
	if !matched {
		return nil, fmt.Errorf("imscore: %s deregistration CSeq %d received mismatched response",
			kind, session.cseq)
	}
	s.recordRegisterResponse(response)
	return response, nil
}

// Unregister removes the current Contact binding from the IMS registrar.
func (s *Service) Unregister(ctx context.Context) error {
	return s.unregisterBindings(ctx, false)
}

// UnregisterAll removes every registrar binding for the current public identity.
func (s *Service) UnregisterAll(ctx context.Context) error {
	return s.unregisterBindings(ctx, true)
}

// releaseSubscriptionsForShutdown ends the event subscriptions on the way out
// but leaves the registrar binding alone. RFC 5626 5.3.1 keys the binding by
// instance-id and reg-id, both stable across a restart here (instance-id is
// the IMEI, and the P-CSCF offers Path;ob), so the next REGISTER replaces it
// rather than adding to it: 43 of 46 registrations reported a single Contact.
// De-registering bought nothing and cost two things. It told the S-CSCF we
// were gone for the whole ~18s a restart needs to rebuild the flow, and its
// Contact expires=0 timed out three times in one day and fell back to
// Contact:*, which third-party de-registers the IP-SM-GW and stopped Vodafone
// pushing MT MESSAGE even after a clean re-REGISTER.
func (s *Service) releaseSubscriptionsForShutdown(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	unsubCtx, cancel := context.WithTimeout(ctx, shutdownUnsubscribeTimeout)
	defer cancel()
	s.unsubscribeMWI(unsubCtx)
	s.unsubscribeRegistration(unsubCtx)
}

func (s *Service) unregisterBindings(ctx context.Context, allBindings bool) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	unsubCtx, unsubCancel := context.WithTimeout(ctx, shutdownUnsubscribeTimeout)
	s.unsubscribeMWI(unsubCtx)
	s.unsubscribeRegistration(unsubCtx)
	unsubCancel()
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	if !s.hasActiveRegistrationForUnregister() {
		return nil
	}
	session, err := s.nextRegisterSession("deregistration")
	if err != nil {
		return err
	}
	response, err := s.exchangeUnregisterAttempt(ctx, session, allBindings)
	if shouldFallbackUnregister(ctx, response, err) {
		s.adoptRegisterSession(session)
		fallbackWildcard := !allBindings
		if fallbackSession, fallbackErr := s.nextRegisterSession("deregistration fallback"); fallbackErr == nil {
			logging.Info("IMS deregistration falling back",
				"device", s.DeviceID(), "cseq", session.cseq, "wildcard", fallbackWildcard, "err", err)
			response, err = s.exchangeUnregisterAttempt(ctx, fallbackSession, fallbackWildcard)
			if err == nil {
				session = fallbackSession
			}
		}
	}
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("imscore: contact deregistration rejected with status %d %s",
			response.StatusCode, strings.TrimSpace(response.Reason))
	}
	s.mu.Lock()
	s.regSession = session
	s.regState = regUnregister
	s.mu.Unlock()
	logging.Info("IMS Contact binding removed", "device", s.DeviceID(), "cseq", session.cseq)
	return nil
}

func registerContactBindingCount(response *sipResponse) int {
	if response == nil {
		return 0
	}
	count := 0
	for _, value := range response.HeaderValues("Contact") {
		for _, part := range splitQuotedSIPHeaderValues(value) {
			if isUEFlowContact(part) {
				count++
			}
		}
	}
	return count
}

func splitQuotedSIPHeaderValues(value string) []string {
	var values []string
	start, angleDepth := 0, 0
	inQuote := false
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '"':
			inQuote = !inQuote
		case '<':
			if !inQuote {
				angleDepth++
			}
		case '>':
			if !inQuote && angleDepth > 0 {
				angleDepth--
			}
		case ',':
			if !inQuote && angleDepth == 0 {
				if item := strings.TrimSpace(value[start:index]); item != "" {
					values = append(values, item)
				}
				start = index + 1
			}
		}
	}
	if item := strings.TrimSpace(value[start:]); item != "" {
		values = append(values, item)
	}
	return values
}

func isUEFlowContact(contact string) bool {
	contact = strings.TrimSpace(contact)
	if contact == "" || contact == "*" {
		return false
	}
	lower := strings.ToLower(contact)
	if strings.Contains(lower, "+sip.instance") || containsSIPParameter(lower, "reg-id") {
		return true
	}
	if containsSIPParameter(lower, "gr") {
		return false
	}
	return true
}

func (s *Service) adoptRegisterSession(session *registerSession) {
	if s == nil || session == nil {
		return
	}
	s.mu.Lock()
	s.regSession = session
	s.mu.Unlock()
}

func (s *Service) exchangeUnregisterAttempt(
	ctx context.Context,
	session *registerSession,
	wildcard bool,
) (*sipResponse, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, deregisterAttemptTimeout(ctx))
	defer cancel()
	return s.exchangeUnregister(attemptCtx, session, wildcard)
}

func shouldFallbackUnregister(ctx context.Context, response *sipResponse, err error) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if err != nil {
		return true
	}
	return response != nil && (response.StatusCode < 200 || response.StatusCode >= 300)
}

func (s *Service) hasActiveRegistrationForUnregister() bool {
	s.mu.RLock()
	registered := s.regState == regRegistered && s.regSession != nil
	s.mu.RUnlock()
	return registered && s.transport != nil && s.transport.hasSendFn()
}
