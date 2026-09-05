package imscore

import (
	"errors"
	"fmt"
	"strings"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

const smsResponseDiagnosticMaxRunes = 256

type smsMESSAGERejection struct {
	status         int
	reason         string
	accept         string
	acceptEncoding string
	warning        string
}

func smsMESSAGERejectionError(response *sip.Response) error {
	if response == nil {
		return errors.New("MESSAGE rejected without a SIP response")
	}
	return formatSMSMESSAGERejection(smsMESSAGERejection{
		status:         response.StatusCode,
		reason:         response.Reason,
		accept:         joinedSIPHeaderValues(response, "Accept"),
		acceptEncoding: joinedSIPHeaderValues(response, "Accept-Encoding"),
		warning:        joinedSIPHeaderValues(response, "Warning"),
	})
}

func internalSMSMESSAGERejectionError(response *sipResponse) error {
	if response == nil {
		return errors.New("MESSAGE rejected without a SIP response")
	}
	return formatSMSMESSAGERejection(smsMESSAGERejection{
		status:         response.StatusCode,
		reason:         response.Reason,
		accept:         strings.Join(response.HeaderValues("Accept"), ", "),
		acceptEncoding: strings.Join(response.HeaderValues("Accept-Encoding"), ", "),
		warning:        strings.Join(response.HeaderValues("Warning"), ", "),
	})
}

func formatSMSMESSAGERejection(rejection smsMESSAGERejection) error {
	message := fmt.Sprintf(
		"MESSAGE rejected with status %d (%s)",
		rejection.status,
		sanitizeSMSResponseValue(rejection.reason),
	)
	for _, detail := range []struct{ name, value string }{
		{name: "accept", value: rejection.accept},
		{name: "accept_encoding", value: rejection.acceptEncoding},
		{name: "warning", value: rejection.warning},
	} {
		if value := sanitizeSMSResponseValue(detail.value); value != "" {
			message += fmt.Sprintf("; %s=%q", detail.name, value)
		}
	}
	return errors.New(message)
}

func joinedSIPHeaderValues(message sip.Message, name string) string {
	headers := message.GetHeaders(name)
	values := make([]string, 0, len(headers))
	for _, header := range headers {
		values = append(values, sipkit.HeaderValue(header, true))
	}
	return strings.Join(values, ", ")
}

func sanitizeSMSResponseValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > smsResponseDiagnosticMaxRunes {
		return string(runes[:smsResponseDiagnosticMaxRunes])
	}
	return value
}
