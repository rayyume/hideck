package voice

import (
	"strconv"
	"strings"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

// HistoryInfoEntry is one TS 24.604 History-Info hi-entry.
type HistoryInfoEntry struct {
	Index string
	URI   string
	Cause int
}

func parseHistoryInfoHeader(header string, request *sip.Request) []HistoryInfoEntry {
	raw := strings.TrimSpace(header)
	if raw == "" && request != nil {
		raw = strings.TrimSpace(requestHeaderValue(request, "History-Info"))
	}
	if raw == "" {
		return nil
	}
	entries, err := parseHistoryInfo(raw)
	if err != nil {
		logging.Info("ignoring malformed History-Info", "header", raw, "err", err)
		return nil
	}
	return entries
}

func parseHistoryInfo(header string) ([]HistoryInfoEntry, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil, nil
	}
	var entries []HistoryInfoEntry
	for _, part := range splitHistoryInfo(header) {
		entry, err := parseHistoryInfoEntry(part)
		if err != nil {
			return nil, err
		}
		if entry.URI != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func splitHistoryInfo(header string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(header); i++ {
		switch header[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(header[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(header[start:]))
	return parts
}

func parseHistoryInfoEntry(part string) (HistoryInfoEntry, error) {
	part = strings.TrimSpace(part)
	if part == "" {
		return HistoryInfoEntry{}, nil
	}
	if strings.Count(part, "<") != strings.Count(part, ">") || !strings.Contains(part, "<") {
		return HistoryInfoEntry{}, errMalformedHistoryInfo
	}
	lt := strings.Index(part, "<")
	gt := strings.Index(part, ">")
	if lt < 0 || gt <= lt {
		return HistoryInfoEntry{}, errMalformedHistoryInfo
	}
	entry := HistoryInfoEntry{URI: strings.TrimSpace(part[lt+1 : gt])}
	if semi := strings.Index(entry.URI, ";"); semi >= 0 {
		applyHistoryInfoParams(&entry, entry.URI[semi+1:])
		entry.URI = strings.TrimSpace(entry.URI[:semi])
	}
	applyHistoryInfoParams(&entry, part[gt+1:])
	return entry, nil
}

func applyHistoryInfoParams(entry *HistoryInfoEntry, raw string) {
	if entry == nil {
		return
	}
	for _, param := range strings.Split(raw, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "index":
			entry.Index = strings.Trim(strings.TrimSpace(value), `"`)
		case "cause", "cause-param":
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil {
				entry.Cause = n
			}
		}
	}
}

func (c *Call) storeHistoryInfo(entries []HistoryInfoEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.historyInfo = append([]HistoryInfoEntry(nil), entries...)
	c.mu.Unlock()
}

// HistoryInfo returns parsed inbound History-Info entries.
func (c *Call) HistoryInfo() []HistoryInfoEntry {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]HistoryInfoEntry(nil), c.historyInfo...)
}

// OriginalCalledURI is the index=1 History-Info URI, or the first entry.
func (c *Call) OriginalCalledURI() string {
	entries := c.HistoryInfo()
	for _, entry := range entries {
		if entry.Index == "1" && entry.URI != "" {
			return entry.URI
		}
	}
	if len(entries) == 0 {
		return ""
	}
	return entries[0].URI
}

var errMalformedHistoryInfo = errString("voice: malformed History-Info")

type errString string

func (e errString) Error() string { return string(e) }
