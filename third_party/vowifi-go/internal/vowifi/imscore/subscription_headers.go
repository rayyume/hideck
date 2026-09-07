package imscore

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

func recordRouteSet(message sip.Message) []string {
	if message == nil {
		return nil
	}
	headers := message.GetHeaders("Record-Route")
	routes := make([]string, 0, len(headers))
	for _, header := range headers {
		if value := strings.TrimSpace(header.Value()); value != "" {
			routes = append(routes, value)
		}
	}
	for i, j := 0, len(routes)-1; i < j; i, j = i+1, j-1 {
		routes[i], routes[j] = routes[j], routes[i]
	}
	return routes
}

func sipAddressTag(value string) string {
	for _, part := range strings.Split(value, ";") {
		name, tag, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(name, "tag") {
			return strings.TrimSpace(tag)
		}
	}
	return ""
}

func subscriptionRefreshDelay(expires time.Duration) time.Duration {
	// TS 24.229 5.1.1.3: 600 seconds early for lifetimes above 1200
	// seconds, otherwise halfway through the negotiated lifetime.
	return registrationRefreshDelay(expires)
}

func subscriptionExpires(response *sip.Response, fallback time.Duration) time.Duration {
	if response == nil {
		return fallback
	}
	value := sipkit.FirstHeaderValue(response, "Expires", true)
	seconds, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func subscriptionRuntimeError(err error) error {
	return fmt.Errorf("imscore: registration event subscription failed: %w", err)
}

func firstSIPHeaderURI(value string) string {
	value = strings.TrimSpace(value)
	if start := strings.IndexByte(value, '<'); start >= 0 {
		if end := strings.IndexByte(value[start+1:], '>'); end >= 0 {
			return strings.TrimSpace(value[start+1 : start+1+end])
		}
	}
	value, _, _ = strings.Cut(value, ",")
	return strings.Trim(strings.TrimSpace(value), "<>")
}
