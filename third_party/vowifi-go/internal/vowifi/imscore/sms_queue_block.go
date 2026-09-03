package imscore

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

// A short message whose report the gateway rejected stays at the head of the
// SMSC queue and holds back every later one until some redelivery is finally
// acknowledged. Measured on 2026-09-03: three rejections over 53 minutes, with
// five messages waiting behind it for up to 48 minutes each. Nothing in the
// logs said so at the time, so the state was only noticed as missing SMS.
const mtQueueBlockTTL = 2 * time.Hour

// mtSMSIdentity keys a short message by what survives redelivery. The MT
// dedup fingerprint cannot be reused: it mixes in the RP-MR, and the SMSC
// allocates a fresh one per delivery attempt, so the same message looks
// different every time it comes back.
func mtSMSIdentity(message inboundSMS) string {
	content := sha256.Sum256([]byte(strings.TrimSpace(message.content)))
	scts := ""
	if !message.timestamp.IsZero() {
		scts = message.timestamp.Truncate(time.Second).UTC().Format(time.RFC3339)
	}
	identity := sha256.Sum256([]byte(strings.Join([]string{
		normalizeFragmentIdentity(message.sender),
		normalizeFragmentIdentity(message.targetURI),
		hex.EncodeToString(content[:]), scts,
	}, "|")))
	return hex.EncodeToString(identity[:])
}

type unacknowledgedMT struct {
	firstRejectedAt time.Time
	rejections      int
}

// rememberRejectedMTReport records that the SMSC still believes this message is
// outstanding, so the next redelivery can be reported as a blocked queue.
func (s *Service) rememberRejectedMTReport(identity string) {
	if s == nil || strings.TrimSpace(identity) == "" {
		return
	}
	now := time.Now()
	s.unackedMTMu.Lock()
	defer s.unackedMTMu.Unlock()
	if s.unackedMT == nil {
		s.unackedMT = make(map[string]unacknowledgedMT, 8)
	}
	for key, entry := range s.unackedMT {
		if now.Sub(entry.firstRejectedAt) > mtQueueBlockTTL {
			delete(s.unackedMT, key)
		}
	}
	entry := s.unackedMT[identity]
	if entry.firstRejectedAt.IsZero() {
		entry.firstRejectedAt = now
	}
	entry.rejections++
	s.unackedMT[identity] = entry
}

// clearRejectedMTReport drops the record once a report for the message lands,
// which is also the moment the SMSC releases whatever queued behind it.
func (s *Service) clearRejectedMTReport(identity string) {
	if s == nil || strings.TrimSpace(identity) == "" {
		return
	}
	s.unackedMTMu.Lock()
	defer s.unackedMTMu.Unlock()
	if entry, held := s.unackedMT[identity]; held {
		delete(s.unackedMT, identity)
		logging.Info("IMS report accepted for a message the SMSC was holding",
			"device", s.DeviceID(), "rejections", entry.rejections,
			"held_for", time.Since(entry.firstRejectedAt).Round(time.Second))
	}
}

// reportMTQueueBlocked runs when a message we could not acknowledge comes back.
// Its redelivery is the observable sign that the SMSC queue has not advanced.
func (s *Service) reportMTQueueBlocked(identity string) {
	if s == nil || strings.TrimSpace(identity) == "" {
		return
	}
	s.unackedMTMu.Lock()
	entry, held := s.unackedMT[identity]
	s.unackedMTMu.Unlock()
	if !held {
		return
	}
	logging.WarnRate("sms-mt-queue-blocked-"+s.DeviceID(), 30*time.Second,
		"IMS SMSC is redelivering a short message whose report it rejected; "+
			"later messages wait behind it",
		"device", s.DeviceID(), "rejections", entry.rejections,
		"blocked_for", time.Since(entry.firstRejectedAt).Round(time.Second))
}
