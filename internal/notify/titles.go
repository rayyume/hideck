package notify

import "strings"

func (c NotificationContext) NotificationTitle() string {
	if title := strings.TrimSpace(c.Title); title != "" {
		return title
	}
	return notificationTitle(c.Event)
}

func notificationTitle(event string) string {
	switch strings.ToLower(strings.TrimSpace(event)) {
	case "sms_received":
		return "收到新短信"
	case "incoming_call":
		return "来电通知"
	case "call_missed":
		return "未接来电"
	case "call_rejected":
		return "已拒接"
	case "call_busy":
		return "忙线"
	case "call_failed":
		return "呼叫失败"
	case "call_completed", "call_ended":
		return "通话结束"
	case "ip_rotated":
		return "公网切换"
	case "wecom_test", "bark_test", "webhook_test":
		return "通知测试"
	default:
		return "HiDeck 通知"
	}
}

func callResultTitle(status, reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "local_hangup":
		return "已挂断"
	case "remote_bye", "remote_hangup":
		return "对方已挂断"
	case "local_reject", "rejected":
		return "已拒接"
	case "remote_cancel":
		return "未接来电"
	case "device_busy", "busy":
		return "忙线"
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "通话结束"
	case "missed":
		return "未接来电"
	case "rejected":
		return "已拒接"
	case "busy":
		return "忙线"
	case "failed":
		return "呼叫失败"
	}
	if title := strings.TrimSpace(status); title != "" {
		return title
	}
	return "通话结束"
}

func notificationLines(title string, fields ...string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(title))
	for i := 0; i+1 < len(fields); i += 2 {
		key := strings.TrimSpace(fields[i])
		value := strings.TrimSpace(fields[i+1])
		if key == "" || value == "" {
			continue
		}
		b.WriteByte('\n')
		b.WriteString(key)
		b.WriteString("    ")
		b.WriteString(value)
	}
	return b.String()
}
