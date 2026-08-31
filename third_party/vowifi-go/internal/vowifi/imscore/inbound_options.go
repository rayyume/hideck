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
	base := "path,sec-agree,outbound,precondition"
	if s != nil && s.cfg != nil {
		if supported := strings.TrimSpace(s.cfg.RegisterTemplate.SupportedHeader); supported != "" {
			return mergeSIPOptionTags(base, supported)
		}
	}
	return base
}

func mergeSIPOptionTags(values ...string) string {
	seen := make(map[string]struct{}, 8)
	out := make([]string, 0, 8)
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			key := strings.ToLower(token)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, token)
		}
	}
	return strings.Join(out, ",")
}
