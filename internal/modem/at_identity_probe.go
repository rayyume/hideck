package modem

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.bug.st/serial"
)

// ATIdentity is the SIM/modem identity readable from a one-shot AT session.
type ATIdentity struct {
	IMEI  string
	IMSI  string
	ICCID string
}

func parseATIdentity(resp string) ATIdentity {
	imei := parseIMEI(resp)
	imsiSrc := resp
	if imei != "" {
		// IMEI is also 15 digits, so strip it before IMSI parse.
		imsiSrc = strings.ReplaceAll(resp, imei, "")
	}
	return ATIdentity{
		IMEI:  imei,
		IMSI:  parseIMSI(imsiSrc),
		ICCID: parseQCCID(resp),
	}
}

func (id ATIdentity) hasSIM() bool {
	return strings.TrimSpace(id.IMSI) != "" || strings.TrimSpace(id.ICCID) != ""
}

// ProbeATIdentity opens the AT port briefly and reads IMEI/IMSI/ICCID without
// starting the long-lived AT manager. Used when QMI DMS/UIM is not up yet.
func ProbeATIdentity(atPort string, timeout time.Duration) (ATIdentity, error) {
	return ProbeATIdentityContext(context.Background(), atPort, timeout)
}

// ProbeATIdentityContext is the cancellable form of ProbeATIdentity.
func ProbeATIdentityContext(ctx context.Context, atPort string, timeout time.Duration) (ATIdentity, error) {
	atPort = strings.TrimSpace(atPort)
	if atPort == "" {
		return ATIdentity{}, errors.New("empty at port")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	p, err := openIMEISerialPort(probeCtx, atPort, mode)
	if err != nil {
		return ATIdentity{}, err
	}
	defer p.Close()
	stopClose := make(chan struct{})
	go closeIMEISerialPortOnCancel(probeCtx, p, stopClose)
	defer close(stopClose)

	_ = p.SetReadTimeout(80 * time.Millisecond)

	write := func(s string) {
		_, _ = p.Write([]byte(s))
	}
	write("AT\r\n")
	time.Sleep(40 * time.Millisecond)
	write("AT+CGSN\r\n")
	time.Sleep(40 * time.Millisecond)
	write("AT+CIMI\r\n")
	time.Sleep(40 * time.Millisecond)
	write("AT+QCCID\r\n")

	buf := make([]byte, 1024)
	var acc strings.Builder
	for probeCtx.Err() == nil {
		n, rerr := p.Read(buf)
		if n > 0 {
			acc.Write(buf[:n])
			ident := parseATIdentity(acc.String())
			if ident.hasSIM() {
				return ident, nil
			}
		}
		if rerr != nil {
			if probeCtx.Err() != nil {
				return ATIdentity{}, probeCtx.Err()
			}
			if strings.Contains(strings.ToLower(rerr.Error()), "timeout") {
				continue
			}
		}
	}

	ident := parseATIdentity(acc.String())
	if ident.hasSIM() {
		return ident, nil
	}
	if err := probeCtx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return ATIdentity{}, err
	}
	return ident, errors.New("at identity probe timeout")
}
