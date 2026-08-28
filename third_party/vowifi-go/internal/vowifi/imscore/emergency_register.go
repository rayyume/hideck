package imscore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/policy"
)

// ErrEmergencyRegistrationDisabled is returned when emergency REGISTER is not opted in.
var ErrEmergencyRegistrationDisabled = errors.New("imscore: emergency registration is disabled")

// BuildEmergencyREGISTER packs a TS 24.229 5.1.6.2 emergency REGISTER.
// It does not send the request and does not mutate the live registration session.
func (s *Service) BuildEmergencyREGISTER(anonymous bool) (string, error) {
	if s == nil || s.cfg == nil {
		return "", errors.New("imscore: service not configured")
	}
	session := s.emergencyRegisterSession()
	return s.buildRegisterRequest(session, "", registerRequestOptions{
		emergency: true, anonymous: anonymous,
	}), nil
}

func (s *Service) emergencyRegisterSession() *registerSession {
	s.mu.RLock()
	current := s.regSession
	s.mu.RUnlock()
	session := &registerSession{
		callID: newCallID(), fromTag: newTag(), contactUser: "emergency-contact",
		cseq: 1, template: resolveActiveIMSRegisterTemplate(s.cfg),
	}
	if current != nil {
		copied := *current
		copied.callID = session.callID
		copied.fromTag = session.fromTag
		copied.cseq = 1
		copied.branch = ""
		session = &copied
	}
	if session.template.ID == "" {
		session.template = policy.DefaultIMSRegisterTemplate()
	}
	if strings.TrimSpace(session.contactUser) == "" {
		session.contactUser = "emergency-contact"
	}
	return session
}

func insertEmergencyContactParam(contact string) string {
	contact = strings.TrimSpace(contact)
	if contact == "" || strings.Contains(strings.ToLower(contact), ";sos") {
		return contact
	}
	if index := strings.Index(contact, ">"); index >= 0 {
		return contact[:index+1] + ";sos" + contact[index+1:]
	}
	return contact + ";sos"
}

// SendEmergencyREGISTER sends a TS 24.229 emergency REGISTER only when the
// host has explicitly opted in. The default path never calls this.
func (s *Service) SendEmergencyREGISTER(ctx context.Context, anonymous bool) error {
	if s == nil || s.cfg == nil || !s.cfg.AllowEmergencyRegistration {
		return ErrEmergencyRegistrationDisabled
	}
	raw, err := s.BuildEmergencyREGISTER(anonymous)
	if err != nil {
		return err
	}
	message, err := parseSIPMessage(raw)
	if err != nil {
		return fmt.Errorf("imscore: parse emergency REGISTER: %w", err)
	}
	request, ok := message.(*sip.Request)
	if !ok {
		return errors.New("imscore: emergency REGISTER is not a SIP request")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	response, _, err := s.dispatchOutboundRequest(ctx, "emergency_register", request, 10*time.Second, true)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("emergency REGISTER rejected with status %d (%s)", response.StatusCode, response.Reason)
	}
	return nil
}
