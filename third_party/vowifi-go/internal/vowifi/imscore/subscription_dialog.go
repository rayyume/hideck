package imscore

import (
	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

func learnSubscriptionDialog(dialog registrationSubscriptionDialog, request *sip.Request, response *sip.Response) registrationSubscriptionDialog {
	if request == nil {
		return dialog
	}
	if request.CallID() != nil {
		if dialog.callID != request.CallID().Value() {
			dialog = registrationSubscriptionDialog{}
		}
		dialog.callID = request.CallID().Value()
	}
	if tag := fromHeaderTag(request.From()); tag != "" {
		dialog.localTag = tag
	}
	if request.CSeq() != nil && request.CSeq().SeqNo > dialog.cseq {
		dialog.cseq = request.CSeq().SeqNo
	}
	if response == nil {
		return dialog
	}
	tag := toHeaderTag(response.To())
	// RFC 6665: NOTIFY establishes the subscription dialog. A late 200
	// from another fork must not replace the first accepted notifier.
	if dialog.notifySeen && tag != dialog.remoteTag {
		return dialog
	}
	if tag != "" {
		dialog.remoteTag = tag
	}
	if target := firstSIPHeaderURI(sipkit.FirstHeaderValue(response, "Contact", true)); target != "" {
		dialog.remoteTarget = target
	}
	if !dialog.notifySeen {
		dialog.routeSet = recordRouteSet(response)
	}
	return dialog
}
