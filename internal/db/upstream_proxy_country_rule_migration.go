package db

import "gorm.io/gorm"

// MigrateUpstreamProxyCountryRuleCompositePK 让同一国家可以绑多条前置代理。
func MigrateUpstreamProxyCountryRuleCompositePK(tx *gorm.DB) error {
	if tx == nil || !tx.Migrator().HasTable(&UpstreamProxyCountryRule{}) {
		return nil
	}
	var rows []UpstreamProxyCountryRule
	if err := tx.Find(&rows).Error; err != nil {
		return err
	}
	if err := tx.Migrator().DropTable(&UpstreamProxyCountryRule{}); err != nil {
		return err
	}
	if err := tx.AutoMigrate(&UpstreamProxyCountryRule{}); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, row := range rows {
		key := row.CountryCode + "\x00" + row.UpstreamProxyID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
