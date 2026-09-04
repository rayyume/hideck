package notify

import (
	"context"
	"strings"

	"github.com/yibaiba/hideck/internal/db"
	"github.com/yibaiba/hideck/internal/phonelookup"
)

func lookupPhoneIdentity(raw string) phonelookup.Result {
	return lookupPhoneIdentityWithRegion(raw, "")
}

func lookupPhoneIdentityWithRegion(raw, region string) phonelookup.Result {
	return db.LookupPhoneIdentityWithRegion(context.Background(), raw, region)
}

func phoneIdentityFields(numberKey, raw string) []string {
	return phoneIdentityFieldsWithRegion(numberKey, raw, "")
}

func phoneIdentityFieldsWithRegion(numberKey, raw, region string) []string {
	id := lookupPhoneIdentityWithRegion(raw, region)
	display := strings.TrimSpace(id.DisplayNumber)
	if display == "" {
		display = strings.TrimSpace(raw)
	}
	fields := []string{numberKey, display}
	if name := strings.TrimSpace(id.Name); name != "" {
		fields = append(fields, "联系人", name)
	}
	if attr := strings.TrimSpace(id.Subtitle); attr != "" {
		fields = append(fields, "归属", attr)
	}
	return fields
}

func fillPhoneIdentityWithRegion(ctx *NotificationContext, raw, region string) {
	if ctx == nil {
		return
	}
	id := lookupPhoneIdentityWithRegion(raw, region)
	if strings.TrimSpace(ctx.Number) == "" {
		ctx.Number = strings.TrimSpace(raw)
	}
	ctx.ContactName = strings.TrimSpace(id.Name)
	ctx.Attribution = strings.TrimSpace(id.Subtitle)
}
