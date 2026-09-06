package db

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type SMSNotifications struct {
	Cursor      uint  `json:"cursor"`
	UnreadCount int64 `json:"unread_count"`
	NewCount    int64 `json:"new_count"`
	Latest      *SMS  `json:"latest,omitempty"`
}

// A missing cursor establishes a baseline without replaying the inbox. IDs,
// not SMSC timestamps, detect delayed messages and keep deletion from replaying
// older receipts. The count and newest preview summarize all new unread SMS.
func GetSMSNotifications(ctx context.Context, afterID *uint) (SMSNotifications, error) {
	var result SMSNotifications
	if DB == nil {
		return result, errors.New("database is not initialized")
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&SMS{}).Where("type = ?", smsTypeIncoming).
			Select("COALESCE(MAX(id), 0)").Scan(&result.Cursor).Error; err != nil {
			return err
		}
		if err := tx.Model(&SMS{}).Where("type = ? AND status = ?", smsTypeIncoming, smsStatusUnread).
			Count(&result.UnreadCount).Error; err != nil {
			return err
		}
		if afterID == nil {
			return nil
		}
		if result.Cursor < *afterID {
			result.Cursor = *afterID
		}
		query := tx.Model(&SMS{}).Where("type = ? AND status = ? AND id > ? AND id <= ?",
			smsTypeIncoming, smsStatusUnread, *afterID, result.Cursor)
		if err := query.Count(&result.NewCount).Error; err != nil || result.NewCount == 0 {
			return err
		}
		var latest SMS
		if err := query.Order("id DESC").First(&latest).Error; err != nil {
			return err
		}
		result.Latest = &latest
		return nil
	})
	return result, err
}
