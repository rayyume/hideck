package notify

import "testing"

func TestNotificationTitleByEvent(t *testing.T) {
	tests := []struct {
		event string
		want  string
	}{
		{event: "sms_received", want: "收到新短信"},
		{event: "incoming_call", want: "来电通知"},
		{event: "call_missed", want: "未接来电"},
		{event: "call_rejected", want: "已拒接"},
		{event: "call_busy", want: "忙线"},
		{event: "call_failed", want: "呼叫失败"},
		{event: "call_completed", want: "通话结束"},
		{event: "call_ended", want: "通话结束"},
		{event: "ip_rotated", want: "公网切换"},
		{event: "wecom_test", want: "通知测试"},
		{event: "", want: "HiDeck 通知"},
	}
	for _, test := range tests {
		if got := notificationTitle(test.event); got != test.want {
			t.Fatalf("notificationTitle(%q) = %q, want %q", test.event, got, test.want)
		}
	}
}

func TestNotificationContextUsesExplicitTitle(t *testing.T) {
	ctx := NotificationContext{Event: "call_completed", Title: "对方已挂断"}
	if got := ctx.NotificationTitle(); got != "对方已挂断" {
		t.Fatalf("title = %q", got)
	}
}

func TestWeComTemplateTitleFollowsCallResult(t *testing.T) {
	values := weComTemplateValues(NotificationContext{
		Event: "call_missed",
		Title: "未接来电",
		Text:  "未接来电\n设备    wwan1\n主叫    18599996654",
	})
	if values["title"] != "未接来电" {
		t.Fatalf("wecom title = %q", values["title"])
	}
}
