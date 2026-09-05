package imscore

import (
	"fmt"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const vodafoneUKMTReportRecoveryPolicy = "vodafone_uk_mt_report_488"

func (s *Service) triggerMTReportPCSCFRecovery(status int) {
	if s == nil || status != 488 || !usesVodafoneUKPortSResetRecovery(s.cfg) || s.stopped() {
		return
	}
	if !s.pcscfRecoveryPending.CompareAndSwap(false, true) {
		return
	}
	go s.requestFreshRuntimeAfterMTReportReject(status)
}

func (s *Service) requestFreshRuntimeAfterMTReportReject(status int) {
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	registrar := s.currentPortSRecoveryRegistrar()
	if registrar == "" || s.stopped() {
		s.pcscfRecoveryPending.Store(false)
		return
	}
	unavailableUntil := time.Now().Add(vodafoneUKPCSCFDeprioritizedPeriod)
	s.registrarPenalties.mark(registrar, unavailableUntil)
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
