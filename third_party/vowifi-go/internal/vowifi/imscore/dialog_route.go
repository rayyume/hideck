package imscore

import (
	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/imsheaders"
)

func recordRouteSetFromMessage(message sip.Message) []string {
	if message == nil {
		return nil
	}
	headers := message.GetHeaders("Record-Route")
	values := make([]string, 0, len(headers))
	for _, header := range headers {
		if value := header.Value(); value != "" {
			values = append(values, value)
		}
	}
	return imsheaders.RecordRouteSet(values)
}

func applyClientDialogRouteSetLocked(dialog *imscoreDialogHandle, response *sip.Response) {
	if dialog == nil {
		return
	}
	if routes := recordRouteSetFromMessage(response); len(routes) > 0 {
		dialog.routeSet = routes
	}
}
