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
	unavailableUntil := time.Now().Add(vodafoneUKPCSCFDeprioritizedPeriod)
	s.registrarPenalties.mark(registrar, unavailableUntil)
	if !s.pcscfRecoveryPending.CompareAndSwap(false, true) {
		return
	}
	go s.requestFreshRuntimeAfterMTReportReject(registrar, status, unavailableUntil)
}

func (s *Service) requestFreshRuntimeAfterMTReportReject(
	registrar string,
	status int,
	unavailableUntil time.Time,
) {
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	current := s.currentPortSRecoveryRegistrar()
	if s.stopped() {
		s.pcscfRecoveryPending.Store(false)
		return
	}
	if !strings.EqualFold(current, registrar) {
		s.pcscfRecoveryPending.Store(false)
		logging.Info("IMS MT report rejection belongs to an earlier P-CSCF path",
			"device", s.DeviceID(), "policy", vodafoneUKMTReportRecoveryPolicy,
			"rejected_pcscf", registrar, "current_pcscf", current,
			"deprioritized_until", unavailableUntil)
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
