package db

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

const smsTargetHistoryLimit = 80

type SMSMessageTarget struct {
	Message  SMS        `json:"message"`
	Contact  SMSContact `json:"contact"`
	Messages []SMS      `json:"messages"`
	HasMore  bool       `json:"has_more"`
}

// The window ends at the target, regardless of its SMSC timestamp or inbox
// page. Reading it does not mark messages as read.
func GetSMSMessageTarget(ctx context.Context, id uint) (SMSMessageTarget, error) {
	var result SMSMessageTarget
	if DB == nil {
		return result, errors.New("database is not initialized")
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&result.Message, id).Error; err != nil {
			return err
		}
		message := result.Message
		if err := tx.Where("iccid = ? AND peer = ?", message.ICCID, message.Peer).First(&result.Contact).Error; err != nil {
			return err
		}
		if err := tx.Where("iccid = ? AND peer = ?", message.ICCID, message.Peer).
			Where("timestamp < ? OR (timestamp = ? AND id <= ?)", message.Timestamp, message.Timestamp, message.ID).
			Order("timestamp DESC, id DESC").Limit(smsTargetHistoryLimit + 1).Find(&result.Messages).Error; err != nil {
			return err
		}
		result.HasMore = len(result.Messages) > smsTargetHistoryLimit
		if result.HasMore {
			result.Messages = result.Messages[:smsTargetHistoryLimit]
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, ErrSMSNotFound
	}
	return result, err
}
