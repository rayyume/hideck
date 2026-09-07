package db

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrSMSReadBoundaryInvalid = errors.New("sms read boundary is invalid")

const (
	smsTypeIncoming = 1
	smsStatusUnread = 0
	smsStatusRead   = 1
)

type SMSReadResult struct {
	Marked      int64
	UnreadCount int
}

type smsReadScope struct {
	throughID  uint
	messageIDs []uint
}

// MarkSMSThreadReadByICCID marks the response snapshot; later inserts stay unread.
func MarkSMSThreadReadByICCID(iccid, peer string, throughID uint) (SMSReadResult, error) {
	if throughID == 0 {
		return SMSReadResult{}, ErrSMSReadBoundaryInvalid
	}
	return markSMSReadByICCID(iccid, peer, smsReadScope{throughID: throughID})
}

// MarkSMSMessagesReadByICCID marks only the displayed historical window.
func MarkSMSMessagesReadByICCID(iccid, peer string, messageIDs []uint) (SMSReadResult, error) {
	if len(messageIDs) == 0 {
		return SMSReadResult{}, ErrSMSReadBoundaryInvalid
	}
	ids := make([]uint, 0, len(messageIDs))
	seen := make(map[uint]bool, len(messageIDs))
	for _, id := range messageIDs {
		if id == 0 {
			return SMSReadResult{}, ErrSMSReadBoundaryInvalid
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return markSMSReadByICCID(iccid, peer, smsReadScope{messageIDs: ids})
}

func markSMSReadByICCID(iccid, peer string, scope smsReadScope) (SMSReadResult, error) {
	iccid = CanonicalICCID(iccid)
	peer = strings.TrimSpace(peer)
	if iccid == "" || peer == "" {
		return SMSReadResult{}, ErrSMSReadBoundaryInvalid
	}
	if DB == nil {
		return SMSReadResult{}, errors.New("database is not initialized")
	}

	var result SMSReadResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var contact SMSContact
		if err := tx.Where("iccid = ? AND peer = ?", iccid, peer).First(&contact).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSMSNotFound
			}
			return err
		}
		if err := scope.validate(tx.Model(&SMS{}).Where("iccid = ? AND peer = ?", iccid, peer)); err != nil {
			return err
		}

		updated := scope.apply(tx.Model(&SMS{})).
			Where("iccid = ? AND peer = ? AND type = ? AND status = ?", iccid, peer, smsTypeIncoming, smsStatusUnread).
			Update("status", smsStatusRead)
		if updated.Error != nil {
			return updated.Error
		}
		result.Marked = updated.RowsAffected

		var unread int64
		if err := tx.Model(&SMS{}).
			Where("iccid = ? AND peer = ? AND type = ? AND status = ?", iccid, peer, smsTypeIncoming, smsStatusUnread).
			Count(&unread).Error; err != nil {
			return err
		}
		result.UnreadCount = int(unread)
		return tx.Model(&SMSContact{}).
			Where("iccid = ? AND peer = ?", iccid, peer).
			Updates(map[string]any{"unread_count": result.UnreadCount, "updated_at": time.Now()}).Error
	})
	return result, err
}

func (scope smsReadScope) apply(query *gorm.DB) *gorm.DB {
	if scope.messageIDs != nil {
		return query.Where("id IN ?", scope.messageIDs)
	}
	return query.Where("id <= ?", scope.throughID)
}

func (scope smsReadScope) validate(query *gorm.DB) error {
	ids := scope.messageIDs
	if ids == nil {
		ids = []uint{scope.throughID}
	}
	var count int64
	if err := query.Where("id IN ?", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return ErrSMSReadBoundaryInvalid
	}
	return nil
}
