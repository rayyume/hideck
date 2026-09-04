package modem

import "testing"

func TestParseATIdentityExtractsIMEIIMSIAndICCID(t *testing.T) {
	resp := "AT+CGSN\r\r\n860000000000001\r\n\r\nOK\r\n" +
		"AT+CIMI\r\r\n460010123456789\r\n\r\nOK\r\n" +
		"AT+QCCID\r\r\n+QCCID: 8986000000000000001\r\n\r\nOK\r\n"
	got := parseATIdentity(resp)
	if got.IMEI != "860000000000001" {
		t.Fatalf("IMEI=%q", got.IMEI)
	}
	if got.IMSI != "460010123456789" {
		t.Fatalf("IMSI=%q", got.IMSI)
	}
	if got.ICCID != "8986000000000000001" {
		t.Fatalf("ICCID=%q", got.ICCID)
	}
	if !got.hasSIM() {
		t.Fatal("hasSIM()=false, want true")
	}
	if !got.hasCompleteSIM() {
		t.Fatal("hasCompleteSIM()=false, want true")
	}
}

func TestParseATIdentityEmptyHasNoSIM(t *testing.T) {
	got := parseATIdentity("\r\nOK\r\n")
	if got.hasSIM() {
		t.Fatalf("hasSIM()=true for empty identity %+v", got)
	}
}

func TestPartialATIdentityIsNotComplete(t *testing.T) {
	got := parseATIdentity("AT+CGSN\r\r\n860000000000001\r\n\r\nOK\r\n" +
		"AT+CIMI\r\r\n460010123456789\r\n\r\nOK\r\n")
	if !got.hasSIM() || got.hasCompleteSIM() {
		t.Fatalf("partial identity completeness is wrong: %+v", got)
	}
}
