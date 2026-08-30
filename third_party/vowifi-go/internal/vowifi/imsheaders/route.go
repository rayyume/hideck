package imsheaders

import (
	"net"
	"strings"

	"github.com/emiago/sipgo/sip"
)

// EffectiveRoute prefers Service-Route and falls back to Path. The two are
// never stacked: RFC 3608 Service-Route is the outbound route set, and Path
// is only used when the registrar did not return Service-Route.
func EffectiveRoute(serviceRoute, path string) string {
	if route := strings.TrimSpace(serviceRoute); route != "" {
		return route
	}
	return strings.TrimSpace(path)
}

// RouteSet returns the non-empty service route and outbound proxy in order.
func RouteSet(serviceRoute, outboundProxy string) []string {
	routes := make([]string, 0, 2)
	if route := strings.TrimSpace(serviceRoute); route != "" {
		routes = append(routes, route)
	}
	if route := strings.TrimSpace(outboundProxy); route != "" {
		routes = append(routes, route)
	}
	return routes
}

// SplitRouteValues splits a stored Route / Service-Route list on commas.
func SplitRouteValues(value string) []string {
	var routes []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			routes = append(routes, item)
		}
	}
	return routes
}

// RecordRouteSet builds the RFC 3261 12.1.2 UAC route set from Record-Route
// header values. Each value may itself be a comma-separated URI list. The
// resulting URIs are taken in reverse order, preserving parameters.
func RecordRouteSet(values []string) []string {
	var uris []string
	for _, value := range values {
		uris = append(uris, SplitRouteValues(value)...)
	}
	for left, right := 0, len(uris)-1; left < right; left, right = left+1, right-1 {
		uris[left], uris[right] = uris[right], uris[left]
	}
	return uris
}

// FirstRoute returns the first effective route.
func FirstRoute(serviceRoute, outboundProxy string) string {
	routes := RouteSet(serviceRoute, outboundProxy)
	if len(routes) == 0 {
		return ""
	}
	return routes[0]
}

// PreloadedOriginatingRoute builds the TS 24.229 5.1.2A.1.1 preloaded Route
// set: P-CSCF (discovery IP and protected server port) first, then the saved
// Service-Route list. Path is not a substitute for Service-Route. A Service-
// Route hop that is already the same URI as the P-CSCF is not duplicated.
func PreloadedOriginatingRoute(pcscf, serviceRoute string) string {
	pcscf = strings.TrimSpace(pcscf)
	routes := make([]string, 0, 4)
	if pcscf != "" {
		routes = append(routes, pcscf)
	}
	for _, item := range SplitRouteValues(serviceRoute) {
		if pcscf != "" && sameRouteHop(item, pcscf) {
			continue
		}
		routes = append(routes, item)
	}
	return strings.Join(routes, ",")
}

func sameRouteHop(left, right string) bool {
	leftURI, leftOK := ParseRouteURI(left)
	rightURI, rightOK := ParseRouteURI(right)
	if !leftOK || !rightOK {
		return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
	}
	return strings.EqualFold(leftURI.Host, rightURI.Host) &&
		routePort(leftURI) == routePort(rightURI) &&
		strings.EqualFold(leftURI.User, rightURI.User)
}

// ParseRouteURI extracts a SIP URI from a Route / Record-Route / Contact value.
func ParseRouteURI(value string) (sip.Uri, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return sip.Uri{}, false
	}
	if start, end := strings.IndexByte(value, '<'), strings.IndexByte(value, '>'); start >= 0 && end > start {
		value = strings.TrimSpace(value[start+1 : end])
	}
	var uri sip.Uri
	if err := sip.ParseUri(value, &uri); err != nil {
		return sip.Uri{}, false
	}
	uri.Host = strings.Trim(strings.TrimSpace(uri.Host), "[]")
	return uri, uri.Host != ""
}

func routePort(uri sip.Uri) int {
	if uri.Port > 0 {
		return uri.Port
	}
	if net.ParseIP(uri.Host) != nil {
		return 5060
	}
	return 0
}
