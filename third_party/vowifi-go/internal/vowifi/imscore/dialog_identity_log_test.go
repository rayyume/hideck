package imscore

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/emiago/sipgo/sip"
)

func TestDialogTokenOfRedactsLongDigits(t *testing.T) {
	got := dialogTokenOf("vohive-447840844894-abc")
	if got.Kind != "redacted" || got.Suffix != "" {
		t.Fatalf("digit-bearing token = %+v", got)
	}
	if got.Length != len("vohive-447840844894-abc") {
		t.Fatalf("redacted length = %d", got.Length)
	}
}

func TestDialogTokenOfKeepsOpaqueSuffix(t *testing.T) {
	got := dialogTokenOf("a5df1a60")
	if got.Kind != "opaque" || got.Length != 8 || got.Suffix != "1a60" {
		t.Fatalf("opaque token = %+v", got)
	}
}

func TestDialogInviteIdentityFieldsMatchHandleAndHideNumbers(t *testing.T) {
	invite := mustClientInviteRequest(t, "vohive-59b005c9505fb58eeda840acee2f0b52")
	response := mustTransactionResponse(t, invite.String(), 200).parsed
	response.AppendHeader(testContactHeader(t, "sip:ab@10.128.120.163:50600;transport=tcp"))
	dialog := newClientDialogHandle(invite, response)
	request := testDialogTemplate(t, sip.INVITE)
	applyDialogRecipient(request, dialog)
	applyDialogCoreHeaders(nil, request, dialog)

	fields := dialogInviteIdentityFields(request, dialog)
	if !dialogIdentityFieldBool(fields, "call_id_eq_handle") ||
		!dialogIdentityFieldBool(fields, "from_tag_eq_handle") ||
		!dialogIdentityFieldBool(fields, "to_tag_eq_handle") ||
		!dialogIdentityFieldBool(fields, "to_tag_eq_response") ||
		!dialogIdentityFieldBool(fields, "ruri_eq_remote_target") {
		t.Fatalf("match fields = %#v", fieldsToMap(fields))
	}
	longDigits := regexp.MustCompile(`\d{8,}`)
	for key, value := range fieldsToMap(fields) {
		if longDigits.MatchString(fmt.Sprint(value)) {
			t.Fatalf("identity field %s leaked digits: %v", key, value)
		}
	}
}

func TestLearnedDialogIdentityFieldsCompareResponseTag(t *testing.T) {
	invite := mustClientInviteRequest(t, "dialog-learn-id")
	response := mustTransactionResponse(t, invite.String(), 183).parsed
	dialog := newClientDialogHandle(invite, response)
	fields := learnedDialogIdentityFields(response, dialog)
	if !dialogIdentityFieldBool(fields, "to_has_tag") || !dialogIdentityFieldBool(fields, "to_tag_eq_handle") {
		t.Fatalf("learned fields = %#v", fieldsToMap(fields))
	}
}

func TestUnmatchedResponseIdentityFieldsIncludeToTag(t *testing.T) {
	invite := mustClientInviteRequest(t, "unmatched-481")
	response := mustTransactionResponse(t, invite.String(), 481)
	fields := unmatchedResponseIdentityFields(response)
	if !dialogIdentityFieldBool(fields, "to_has_tag") {
		t.Fatalf("unmatched fields = %#v", fieldsToMap(fields))
	}
}

func fieldsToMap(fields []any) map[string]any {
	out := make(map[string]any, len(fields)/2)
	for index := 0; index+1 < len(fields); index += 2 {
		key, _ := fields[index].(string)
		out[key] = fields[index+1]
	}
	return out
}

func dialogIdentityFieldBool(fields []any, name string) bool {
	value, ok := fieldsToMap(fields)[name].(bool)
	return ok && value
}

func TestDialogTokenFieldsNeverPrintMSISDN(t *testing.T) {
	fields := appendDialogTokenFields(nil, "call_id", dialogTokenOf("+447840844894"))
	for _, value := range fields {
		if strings.Contains(fmt.Sprint(value), "447840844894") || strings.Contains(fmt.Sprint(value), "4894") {
			t.Fatalf("token fields leaked number: %#v", fields)
		}
	}
}
