package imscore

import (
	"fmt"
	"strings"
)

const inboundSIPAllowHeader = "OPTIONS, NOTIFY, INVITE, ACK, BYE, CANCEL, UPDATE, PRACK, REFER, INFO, MESSAGE"

const inboundOPTIONSAccept = "application/sdp, application/reginfo+xml, application/simple-message-summary"

func (s *Service) buildInboundOPTIONSResponse(request string) (string, error) {
	headers, err := inboundResponseHeaders(request, newTag())
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "SIP/2.0 200 OK\r\n%s", headers)
	if allow := s.inboundOPTIONSAllow(); allow != "" {
		fmt.Fprintf(&out, "Allow: %s\r\n", allow)
	}
	if supported := s.inboundOPTIONSSupported(); supported != "" {
		fmt.Fprintf(&out, "Supported: %s\r\n", supported)
	}
	fmt.Fprintf(&out, "Accept: %s\r\n", inboundOPTIONSAccept)
	fmt.Fprintf(&out, "Content-Length: 0\r\n\r\n")
	return out.String(), nil
}

func (s *Service) inboundOPTIONSAllow() string {
	return inboundSIPAllowHeader
}

func (s *Service) inboundOPTIONSSupported() string {
	if s != nil && s.cfg != nil {
		if supported := strings.TrimSpace(registerSupportedHeader(s.cfg)); supported != "" {
			return supported
		}
	}
	return "path,sec-agree,outbound"
}
