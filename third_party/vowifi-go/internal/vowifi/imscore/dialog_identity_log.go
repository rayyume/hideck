package imscore

import (
	"regexp"
	"strings"

	"github.com/emiago/sipgo/sip"
)

var dialogLongDigitPattern = regexp.MustCompile(`\d{8,}`)

type dialogTokenFingerprint struct {
	Kind   string
	Length int
	Suffix string
}

func dialogTokenOf(value string) dialogTokenFingerprint {
	value = strings.TrimSpace(value)
	if value == "" {
		return dialogTokenFingerprint{Kind: "empty"}
	}
	if dialogLongDigitPattern.MatchString(value) {
		return dialogTokenFingerprint{Kind: "redacted", Length: len(value)}
	}
	suffix := value
	if len(value) > 4 {
		suffix = value[len(value)-4:]
	}
	return dialogTokenFingerprint{Kind: "opaque", Length: len(value), Suffix: suffix}
}

func appendDialogTokenFields(fields []any, prefix string, token dialogTokenFingerprint) []any {
	fields = append(fields, prefix+"_kind", token.Kind, prefix+"_len", token.Length)
	if token.Kind == "opaque" && token.Suffix != "" {
		fields = append(fields, prefix+"_suffix", token.Suffix)
	}
	return fields
}

func dialogURIUserFields(prefix string, uri sip.Uri) []any {
	return []any{
		prefix + "_user_kind", dialogRequestURIUserKind(uri),
		prefix + "_user_shape", dialogContactUserShape(uri.User),
		prefix + "_user_len", len(strings.TrimSpace(uri.User)),
		prefix + "_host", strings.ToLower(strings.Trim(uri.Host, "[]")),
		prefix + "_port", uri.Port,
	}
}

func dialogRequestToTag(request *sip.Request) string {
	if request == nil || request.To() == nil {
		return ""
	}
	return toHeaderTag(request.To())
}

func dialogRequestFromTag(request *sip.Request) string {
	if request == nil || request.From() == nil {
		return ""
	}
	return fromHeaderTag(request.From())
}

func sipURISameTarget(left, right sip.Uri) bool {
	if strings.ToLower(strings.Trim(left.Host, "[]")) != strings.ToLower(strings.Trim(right.Host, "[]")) {
		return false
	}
	if strings.TrimSpace(left.User) != strings.TrimSpace(right.User) {
		return false
	}
	return left.Port == right.Port
}

func dialogInviteIdentityFields(request *sip.Request, handle *imscoreDialogHandle) []any {
	fields := []any{}
	if request != nil {
		fields = append(fields,
			"ruri_user_shape", dialogContactUserShape(request.Recipient.User),
			"ruri_user_len", len(strings.TrimSpace(request.Recipient.User)),
		)
		if request.To() != nil {
			fields = append(fields, dialogURIUserFields("to", request.To().Address)...)
		}
		fields = appendDialogTokenFields(fields, "call_id", dialogTokenOf(requestCallID(request)))
		fields = appendDialogTokenFields(fields, "from_tag", dialogTokenOf(dialogRequestFromTag(request)))
		fields = appendDialogTokenFields(fields, "to_tag", dialogTokenOf(dialogRequestToTag(request)))
		fields = append(fields, "to_has_tag", dialogRequestToTag(request) != "")
	}
	if handle == nil {
		return fields
	}
	handle.mu.Lock()
	callID := handle.callID
	fromTag := handle.fromTag
	toTag := handle.toTag
	confirmed := handle.confirmed
	remote := *handle.remoteTarget.Clone()
	responseToTag := ""
	responseTo := sip.Uri{}
	if handle.inviteResponse != nil && handle.inviteResponse.To() != nil {
		responseToTag = toHeaderTag(handle.inviteResponse.To())
		responseTo = handle.inviteResponse.To().Address
	}
	inviteFromTag := ""
	if handle.inviteRequest != nil && handle.inviteRequest.From() != nil {
		inviteFromTag = fromHeaderTag(handle.inviteRequest.From())
	}
	handle.mu.Unlock()

	fields = append(fields, "handle_confirmed", confirmed)
	fields = append(fields, dialogURIUserFields("remote_target", remote)...)
	fields = appendDialogTokenFields(fields, "handle_call_id", dialogTokenOf(callID))
	fields = appendDialogTokenFields(fields, "handle_from_tag", dialogTokenOf(fromTag))
	fields = appendDialogTokenFields(fields, "handle_to_tag", dialogTokenOf(toTag))
	if request != nil {
		fields = append(fields,
			"call_id_eq_handle", requestCallID(request) == callID,
			"from_tag_eq_handle", dialogRequestFromTag(request) == fromTag,
			"to_tag_eq_handle", dialogRequestToTag(request) == toTag,
			"to_tag_eq_response", dialogRequestToTag(request) == responseToTag,
			"from_tag_eq_invite", dialogRequestFromTag(request) == inviteFromTag,
			"ruri_eq_remote_target", sipURISameTarget(request.Recipient, remote),
		)
		if request.To() != nil {
			fields = append(fields, "to_uri_eq_response", sipURISameTarget(request.To().Address, responseTo))
		}
	}
	return fields
}

func learnedDialogIdentityFields(response *sip.Response, dialog *imscoreDialogHandle) []any {
	fields := []any{}
	if response != nil {
		if response.CallID() != nil {
			fields = appendDialogTokenFields(fields, "call_id", dialogTokenOf(response.CallID().Value()))
		}
		if response.From() != nil {
			fields = append(fields, dialogURIUserFields("from", response.From().Address)...)
			fields = appendDialogTokenFields(fields, "from_tag", dialogTokenOf(fromHeaderTag(response.From())))
		}
		if response.To() != nil {
			fields = append(fields, dialogURIUserFields("to", response.To().Address)...)
			fields = appendDialogTokenFields(fields, "to_tag", dialogTokenOf(toHeaderTag(response.To())))
			fields = append(fields, "to_has_tag", toHeaderTag(response.To()) != "")
		}
	}
	if dialog == nil {
		return fields
	}
	dialog.mu.Lock()
	handleToTag := dialog.toTag
	confirmed := dialog.confirmed
	dialog.mu.Unlock()
	responseToTag := ""
	if response != nil && response.To() != nil {
		responseToTag = toHeaderTag(response.To())
	}
	fields = append(fields,
		"handle_confirmed", confirmed,
		"to_tag_eq_handle", responseToTag == handleToTag,
	)
	fields = appendDialogTokenFields(fields, "handle_to_tag", dialogTokenOf(handleToTag))
	return fields
}

func unmatchedResponseIdentityFields(response *sipResponse) []any {
	if response == nil {
		return nil
	}
	fields := []any{"status", response.StatusCode, "cseq", response.CSeq}
	fields = appendDialogTokenFields(fields, "call_id", dialogTokenOf(response.CallID))
	if response.parsed != nil && response.parsed.To() != nil {
		fields = appendDialogTokenFields(fields, "to_tag", dialogTokenOf(toHeaderTag(response.parsed.To())))
		fields = append(fields, "to_has_tag", toHeaderTag(response.parsed.To()) != "")
	}
	if response.parsed != nil && response.parsed.From() != nil {
		fields = appendDialogTokenFields(fields, "from_tag", dialogTokenOf(fromHeaderTag(response.parsed.From())))
	}
	return fields
}
