package db

import (
	"strings"

	"gorm.io/gorm"
)

// MigratePhoneContactIDs gives legacy number-only rows independent identities.
// Existing names are not used because equal names do not prove equal people.
func MigratePhoneContactIDs(database *gorm.DB) error {
	if database == nil || !database.Migrator().HasTable(&PhoneContact{}) {
		return nil
	}
	return database.Transaction(func(tx *gorm.DB) error {
		var rows []PhoneContact
		if err := tx.Where("TRIM(COALESCE(contact_id, '')) = ''").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			id, err := newPhoneContactID()
			if err != nil {
				return err
			}
			if strings.TrimSpace(row.Number) == "" {
				continue
			}
			if err := tx.Model(&PhoneContact{}).Where("number = ?", row.Number).Update("contact_id", id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
