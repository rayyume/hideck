package swu

import (
	"time"

	"github.com/iniwex5/vowifi-go/engine/ikev2"
	"github.com/iniwex5/vowifi-go/engine/logger"
	"go.uber.org/zap"
)

const defaultReauthOverlapGrace = 10 * time.Second

func (s *Session) runtimeRedirectAddress(payloads []ikev2.Payload) string {
	for _, payload := range payloads {
		notify, ok := payload.(*ikev2.EncryptedPayloadNotify)
		if !ok || notify.NotifyType != ikev2.REDIRECT {
			continue
		}
		address, err := ParseRedirectData(notify.NotifyData)
		if err != nil {
			s.Logger.Warn("invalid runtime REDIRECT notification", zap.Error(err))
			return ""
		}
		return address
	}
	return ""
}

func (s *Session) handleRuntimeRedirect(address string) {
	if address == "" {
		return
	}
	logger.Warn("SWu runtime REDIRECT received", zap.String("target", address))
	if s.OnRedirect != nil {
		go s.OnRedirect(address)
	}
	if s.OnSessionDown != nil {
		s.notifySessionDown()
		return
	}
	s.cancel()
}

func (s *Session) triggerReauthentication() {
	// RFC 7296 2.8.3: reauthentication starts a new IKE_SA_INIT/IKE_AUTH.
	// The host creates that runtime via OnReauthNeeded. This session never
	// injects EAP onto the existing IKE SA.
	if s.OnReauthNeeded != nil {
		go s.OnReauthNeeded()
	}
	grace := s.reauthOverlapGrace
	if grace <= 0 {
		grace = defaultReauthOverlapGrace
	}
	s.armTimer(&s.ikeReauthTimer, grace, func() {
		if s.ctx.Err() == nil {
			s.finishReauthentication()
		}
	})
}

func (s *Session) finishReauthentication() {
	s.sendEstablishedDeletes()
	s.failEstablishedControl(ErrFreshRuntimeRequired)
}
