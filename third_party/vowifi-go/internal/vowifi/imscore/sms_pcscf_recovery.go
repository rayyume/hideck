package imscore

import (
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const vodafoneUKMTReportRecoveryPolicy = "vodafone_uk_mt_report_488"

func (s *Service) triggerMTReportPCSCFRecovery(reportErr error) {
	status := rpReportRejectStatus(reportErr)
	registrar := rpReportRejectRegistrar(reportErr)
	if s == nil || status != 488 || registrar == "" ||
		!usesVodafoneUKPortSResetRecovery(s.cfg) || s.stopped() {
		return
	}
	penalty := s.markVodafoneRegistrarFailure(registrar, "mt_report_488", nil)
	if !s.pcscfRecoveryPending.CompareAndSwap(false, true) {
		return
	}
	go s.recoverPCSCFAfterMTReportReject(registrar, status, penalty.deprioritizedUntil)
}

func (s *Service) recoverPCSCFAfterMTReportReject(
	registrar string,
	status int,
	unavailableUntil time.Time,
) {
	defer s.finishPCSCFRecovery()
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	current := s.currentPortSRecoveryRegistrar()
	if s.stopped() {
		return
	}
	if !strings.EqualFold(current, registrar) {
		logging.Info("IMS MT report rejection belongs to an earlier P-CSCF path",
			"device", s.DeviceID(), "policy", vodafoneUKMTReportRecoveryPolicy,
			"rejected_pcscf", registrar, "current_pcscf", current,
			"deprioritized_until", unavailableUntil)
		return
	}
	s.registrarPenalties.resetDownlinkRound(registrar)
	s.mu.Lock()
	next := s.advanceAvailableRegistrarLocked()
	s.mu.Unlock()
	cause := portSFailoverCause{
		reason: "mt_report_488", observedAt: time.Now(), deprioritizedUntil: unavailableUntil,
	}
	if next != "" {
		s.recoverPortSOnAlternate(registrar, next, cause)
		return
	}
	reason := fmt.Sprintf("P-CSCF path %s rejected the MT SMS RP report with SIP %d", registrar, status)
	s.markPCSCFRegistrationUnboundWithReason(reason, int32(status), SIPStatusText(status))
	err := fmt.Errorf("imscore: %s; fresh runtime required", reason)
	logging.WarnRate("ims-mt-report-pcscf-recovery-"+s.DeviceID()+"-"+registrar, 30*time.Second,
		"IMS MT report rejection requires a fresh P-CSCF path",
		"device", s.DeviceID(), "policy", vodafoneUKMTReportRecoveryPolicy,
		"pcscf", registrar, "status", status,
		"deprioritized_until", unavailableUntil)
	s.reportRegistrationRuntimeError(err)
}
