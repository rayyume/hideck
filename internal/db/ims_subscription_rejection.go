package db

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const maxIMSSubscriptionIdentityBytes = 4096

type IMSSubscriptionRejection struct {
	IdentityHash string    `gorm:"column:identity_hash;primaryKey;size:64"`
	EventPackage string    `gorm:"column:event_package;primaryKey;size:32"`
	StatusCode   int       `gorm:"column:status_code;not null"`
	ExpiresAt    time.Time `gorm:"column:expires_at;index;not null"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func LoadIMSSubscriptionRejection(
	identity, eventPackage string,
	now time.Time,
) (int, time.Time, error) {
	identityHash, eventPackage, err := normalizeIMSSubscriptionRejectionKey(identity, eventPackage)
	if err != nil {
		return 0, time.Time{}, err
	}
	if DB == nil {
		return 0, time.Time{}, gorm.ErrInvalidDB
	}
	var row IMSSubscriptionRejection
	err = DB.Where("identity_hash = ? AND event_package = ?", identityHash, eventPackage).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, time.Time{}, nil
		}
		return 0, time.Time{}, err
	}
	if !now.Before(row.ExpiresAt) {
		err = DB.Delete(&row).Error
		return 0, time.Time{}, err
	}
	if row.StatusCode == 405 {
		if err := DB.Delete(&row).Error; err != nil {
			return 0, time.Time{}, err
		}
		return 0, time.Time{}, nil
	}
	if !validPersistentSubscriptionStatus(row.StatusCode) {
		return 0, time.Time{}, fmt.Errorf("invalid persisted IMS subscription status %d", row.StatusCode)
	}
	return row.StatusCode, row.ExpiresAt, nil
}

func SaveIMSSubscriptionRejection(
	identity, eventPackage string,
	status int,
	expiresAt time.Time,
) error {
	identityHash, eventPackage, err := normalizeIMSSubscriptionRejectionKey(identity, eventPackage)
	if err != nil {
		return err
	}
	if !validPersistentSubscriptionStatus(status) {
		return fmt.Errorf("IMS subscription status %d is not persistable", status)
	}
	if expiresAt.IsZero() {
		return fmt.Errorf("IMS subscription rejection expiration is required")
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return upsertIMSSubscriptionRejection(tx, IMSSubscriptionRejection{
			IdentityHash: identityHash, EventPackage: eventPackage,
			StatusCode: status, ExpiresAt: expiresAt,
		})
	})
}

func DeleteIMSSubscriptionRejections(identity string) error {
	identityHash, _, err := normalizeIMSSubscriptionRejectionKey(identity, "reg")
	if err != nil {
		return err
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	return DB.Where("identity_hash = ?", identityHash).Delete(&IMSSubscriptionRejection{}).Error
}

func upsertIMSSubscriptionRejection(tx *gorm.DB, row IMSSubscriptionRejection) error {
	var current IMSSubscriptionRejection
	err := tx.Where(
		"identity_hash = ? AND event_package = ?", row.IdentityHash, row.EventPackage,
	).First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&row).Error
	}
	if err != nil {
		return err
	}
	if current.ExpiresAt.After(row.ExpiresAt) {
		row.ExpiresAt = current.ExpiresAt
	}
	return tx.Model(&current).Updates(map[string]any{
		"status_code": row.StatusCode,
		"expires_at":  row.ExpiresAt,
	}).Error
}

func normalizeIMSSubscriptionRejectionKey(identity, eventPackage string) (string, string, error) {
	identity = strings.TrimSpace(identity)
	if identity == "" || len(identity) > maxIMSSubscriptionIdentityBytes {
		return "", "", fmt.Errorf("invalid IMS subscription identity length %d", len(identity))
	}
	eventPackage = strings.ToLower(strings.TrimSpace(eventPackage))
	if eventPackage != "reg" && eventPackage != "message-summary" {
		return "", "", fmt.Errorf("unsupported IMS event package %q", eventPackage)
	}
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:]), eventPackage, nil
}

func validPersistentSubscriptionStatus(status int) bool {
	return status == 489
}
