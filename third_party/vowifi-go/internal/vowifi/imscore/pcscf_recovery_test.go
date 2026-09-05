package imscore

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imsendpoint"
)

func TestDecidePCSCF503RecoveryFollowsTimerB(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	timerB := 32 * time.Second
	defaultPenalty := 20 * time.Second
	tests := []struct {
		name       string
		retryAfter string
		want       bool
		wantUntil  time.Time
	}{
		{name: "absent", want: true, wantUntil: now.Add(defaultPenalty)},
		{name: "short", retryAfter: "10", want: false},
		{name: "equal", retryAfter: "32", want: false},
		{name: "long", retryAfter: "60 (maintenance)", want: true, wantUntil: now.Add(time.Minute)},
		// Long Retry-After still failovers because waiting past Timer B would
		// strand the originating INVITE. IR.92 2.2.1 only mandates this when
		// Retry-After is absent; this extra branch is documented tolerance.
		{name: "invalid", retryAfter: "later", want: true, wantUntil: now.Add(defaultPenalty)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &sipResponse{StatusCode: 503}
			if test.retryAfter != "" {
				response.Headers = map[string]string{"Retry-After": test.retryAfter}
			}
			decision := decidePCSCF503Recovery(pcscf503RecoveryInput{
				response: response, timerB: timerB, now: now, defaultPenalty: defaultPenalty,
			})
			if decision.recover != test.want || !decision.unavailableUntil.Equal(test.wantUntil) {
				t.Fatalf("decision = %+v, want recover=%t until=%s", decision, test.want, test.wantUntil)
			}
		})
	}
}

func TestPCSCFUnavailableCandidatesAreSkipped(t *testing.T) {
	service := newRegisteredClientInviteService(t)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{
		"pcscf-a.example:5060", "pcscf-b.example:5060", "pcscf-c.example:5060",
	}
	service.registrarPenalties.mark("pcscf-b.example:5060", time.Now().Add(time.Minute))
	service.mu.Unlock()

	next, current := service.markRegistrarUnavailableAndAdvance(
		"pcscf-a.example:5060", time.Now().Add(time.Minute),
	)
	if !current || next != "pcscf-c.example:5060" {
		t.Fatalf("advance = %q current=%t", next, current)
	}
	if service.advanceRegistrarForNextRetry("network") {
		t.Fatal("retry selected a P-CSCF that is still unavailable")
	}
}

func TestPCSCFPenaltySurvivesServiceReplacement(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	now := time.Now()
	store.mark("pcscf-a.example:5060", now.Add(30*time.Minute))

	service, err := New(&IMSConfig{
		Registrar: "pcscf-a.example:5060;pcscf-b.example:5060",
		LocalAddr: "192.0.2.10", RegistrarPenalties: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := service.selectRegistrarCandidate(context.Background(), "udp")
	if err != nil {
		t.Fatal(err)
	}
	if selected != "pcscf-b.example:5060" {
		t.Fatalf("selected P-CSCF = %q, want unpenalized alternate", selected)
	}
	if _, exists := service.StatusCurrent().DeprioritizedPCSCF["pcscf-a.example:5060"]; !exists {
		t.Fatal("replacement service lost the active P-CSCF penalty")
	}
}

func TestPCSCFSelectionRejectsAllPenalizedCandidates(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	now := time.Now()
	firstUntil := now.Add(20 * time.Minute)
	store.mark("pcscf-a.example:5060", firstUntil)
	store.mark("pcscf-b.example:5060", now.Add(30*time.Minute))
	service, err := New(&IMSConfig{
		Registrar: "pcscf-a.example:5060;pcscf-b.example:5060",
		LocalAddr: "192.0.2.10", RegistrarPenalties: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.selectRegistrarCandidate(context.Background(), "udp")
	var unavailable *allRegistrarCandidatesUnavailableError
	if err == nil || !errors.As(err, &unavailable) ||
		!strings.Contains(err.Error(), "temporarily unavailable") {
		t.Fatalf("selection error = %v", err)
	}
	if !unavailable.RetryAt().Equal(firstUntil) {
		t.Fatalf("next P-CSCF retry = %s, want %s", unavailable.RetryAt(), firstUntil)
	}
}

func TestPCSCFShorterPenaltyDoesNotReplaceLongerPenalty(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	now := time.Now()
	longer := now.Add(time.Hour)
	store.mark("pcscf-a.example:5060", longer)
	store.mark("pcscf-a.example:5060", now.Add(30*time.Minute))
	if got := store.snapshot(now)["pcscf-a.example:5060"]; !got.Equal(longer) {
		t.Fatalf("penalty deadline = %s, want %s", got, longer)
	}
}

func TestPCSCFZeroPenaltyDoesNotPermanentlyExcludeRegistrar(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	store.mark("pcscf-a.example:5060", time.Time{})
	if store.unavailable("pcscf-a.example:5060", time.Now()) {
		t.Fatal("zero penalty permanently excluded the P-CSCF")
	}
}

func TestPCSCFFailureCountSurvivesPenaltyExpiryAndServiceReplacement(t *testing.T) {
	const registrar = "pcscf-a.example:5060"
	store := NewRegistrarPenaltyStore()
	now := time.Unix(1_700_000_000, 0)
	if got := store.recordFailure(registrar); got != 1 {
		t.Fatalf("first failure count = %d, want 1", got)
	}
	store.mark(registrar, now.Add(time.Minute))
	if store.unavailable(registrar, now.Add(2*time.Minute)) {
		t.Fatal("expired P-CSCF penalty remained active")
	}

	service, err := New(&IMSConfig{
		Registrar: registrar, LocalAddr: "192.0.2.10", RegistrarPenalties: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.StopCurrent)
	if got := service.registrarPenalties.recordFailure(registrar); got != 2 {
		t.Fatalf("replacement service failure count = %d, want 2", got)
	}
}

func TestProtectedDownlinkClearsPCSCFFailureCount(t *testing.T) {
	const registrar = "pcscf-a.example:5060"
	store := NewRegistrarPenaltyStore()
	store.recordFailure(registrar)
	store.mark(registrar, time.Now().Add(time.Minute))
	service, err := New(&IMSConfig{
		Registrar: registrar, LocalAddr: "192.0.2.10", RegistrarPenalties: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.StopCurrent)
	service.mu.Lock()
	service.registrar = registrar
	service.mu.Unlock()
	client, server := net.Pipe()
	defer server.Close()
	if !service.trackProtectedConnection(client) {
		t.Fatal("track protected connection")
	}
	if !store.unavailable(registrar, time.Now()) {
		t.Fatal("proven port-s removed an active P-CSCF penalty")
	}
	if got := store.recordFailure(registrar); got != 1 {
		t.Fatalf("failure count after proven port-s = %d, want 1", got)
	}
}

func TestPCSCFSwitchDropsOldSecurityAgreement(t *testing.T) {
	network := &removableCaptureNetwork{
		captureIPSecNetwork: &captureIPSecNetwork{SystemIMSNetwork: NewSystemIMSNetwork(testLocalIP)},
	}
	service := newSecurityAgreementTestService(t, network)
	service.mu.Lock()
	service.spiPairs = [][2]uint32{{1, 2}}
	service.regSession = &registerSession{
		expires:  time.Hour,
		security: &securityAgreement{server: &securityMechanism{Name: "ipsec-3gpp"}},
	}
	service.serviceRoute = "<sip:old-route.example;lr>"
	service.path = "<sip:old-path.example;lr>"
	service.securityVerify = "ipsec-3gpp;spi-c=1;spi-s=2"
	service.outboundContactRegistered = true
	service.mu.Unlock()
	client, server := net.Pipe()
	defer server.Close()
	if !service.trackProtectedConnection(client) {
		t.Fatal("track protected connection")
	}

	if err := service.resetRegistrationForPCSCFSwitch(); err != nil {
		t.Fatal(err)
	}
	service.mu.RLock()
	regSession := service.regSession
	serviceRoute := service.serviceRoute
	path := service.path
	securityVerify := service.securityVerify
	outboundRegistered := service.outboundContactRegistered
	service.mu.RUnlock()
	if network.removals != 1 || regSession != nil || serviceRoute != "" ||
		path != "" || securityVerify != "" || outboundRegistered {
		t.Fatalf("old P-CSCF state survived reset: removals=%d session=%+v route=%q path=%q verify=%q outbound=%t",
			network.removals, regSession, serviceRoute, path, securityVerify, outboundRegistered)
	}
	service.protectedConnMu.Lock()
	protectedConnectionCount := len(service.protectedConns)
	service.protectedConnMu.Unlock()
	if service.portSPushReady.Load() || protectedConnectionCount != 0 {
		t.Fatal("old port-s connection remained reachable during P-CSCF switch")
	}
}

func TestInitialInvite503WithoutAlternateInvalidatesRegistration(t *testing.T) {
	service := newRegisteredClientInviteService(t)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{"pcscf-a.example:5060"}
	service.signalingReady = true
	service.mu.Unlock()
	outbound := recordTransactionWrites(service.transport)
	request := mustClientInviteRequest(t, "invite-503-recovery")
	result := make(chan legacyClientInviteOutcome, 1)
	go func() {
		value, err := service.StartClientInvite(t.Context(), service.DeviceID(), imsendpoint.ClientInviteOptions{
			Request: request, Contact: request.Contact(),
		})
		result <- legacyClientInviteOutcome{result: value, err: err}
	}()

	written := waitTransactionWrite(t, outbound)
	service.transport.DeliverResponse(mustTransactionResponse(t, written, 503))
	outcome := <-result
	if outcome.result == nil || outcome.result.Response == nil || outcome.result.Response.StatusCode != 503 {
		t.Fatalf("result = %+v", outcome.result)
	}
	if outcome.err == nil || !strings.Contains(outcome.err.Error(), "503") {
		t.Fatalf("error = %v", outcome.err)
	}
	assertPCSCFRecoveryRequested(t, service)
}

func TestCanceledInitialInviteRetains503ForPCSCFRecovery(t *testing.T) {
	service := newRegisteredClientInviteService(t)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{"pcscf-a.example:5060"}
	service.signalingReady = true
	service.mu.Unlock()
	outbound := recordTransactionWrites(service.transport)
	request := mustClientInviteRequest(t, "canceled-invite-503-recovery")
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan legacyClientInviteOutcome, 1)
	go func() {
		value, err := service.StartClientInvite(ctx, service.DeviceID(), imsendpoint.ClientInviteOptions{
			Request: request, Contact: request.Contact(),
		})
		result <- legacyClientInviteOutcome{result: value, err: err}
	}()

	writtenInvite := waitTransactionWrite(t, outbound)
	service.transport.DeliverResponse(mustTransactionResponse(t, writtenInvite, 100))
	cancel()
	writtenCancel := waitForTransactionMethod(t, outbound, "CANCEL")
	service.transport.DeliverResponse(mustTransactionResponse(t, writtenInvite, 503))
	service.transport.DeliverResponse(mustTransactionResponse(t, writtenCancel, 481))
	outcome := <-result
	if !errors.Is(outcome.err, context.Canceled) || outcome.result == nil ||
		outcome.result.Response == nil || outcome.result.Response.StatusCode != 503 {
		t.Fatalf("result=%+v error=%v", outcome.result, outcome.err)
	}
	assertPCSCFRecoveryRequested(t, service)
}

func assertPCSCFRecoveryRequested(t *testing.T, service *Service) {
	t.Helper()
	select {
	case err := <-service.RegistrationErrors():
		if err == nil || !strings.Contains(err.Error(), "no alternate is available") {
			t.Fatalf("runtime recovery error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("503 did not request P-CSCF rediscovery")
	}
	if service.IsRegistered() {
		t.Fatal("503 left the failed P-CSCF registration active")
	}
}
