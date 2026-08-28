package imsheaders

import "strings"

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

// FirstRoute returns the first effective route.
func FirstRoute(serviceRoute, outboundProxy string) string {
	routes := RouteSet(serviceRoute, outboundProxy)
	if len(routes) == 0 {
		return ""
	}
	return routes[0]
}
