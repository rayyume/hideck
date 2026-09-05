package db

import (
	"sort"
	"strings"

	"gorm.io/gorm"
)

// MigrateUpstreamProxyCountryRuleCompositePK 让同一国家可以绑多条前置代理。
func MigrateUpstreamProxyCountryRuleCompositePK(database *gorm.DB) error {
	if database == nil || !database.Migrator().HasTable(&UpstreamProxyCountryRule{}) {
		return nil
	}
	primaryKeys, err := upstreamProxyCountryRulePrimaryKeys(database)
	if err != nil {
		return err
	}
	if strings.Join(primaryKeys, ",") == "country_code,upstream_proxy_id" {
		return nil
	}
	return database.Transaction(rebuildUpstreamProxyCountryRules)
}

func upstreamProxyCountryRulePrimaryKeys(database *gorm.DB) ([]string, error) {
	type columnInfo struct {
		Name string `gorm:"column:name"`
		PK   int    `gorm:"column:pk"`
	}
	var columns []columnInfo
	if err := database.Raw("PRAGMA table_info('upstream_proxy_country_rules')").Scan(&columns).Error; err != nil {
		return nil, err
	}
	primaryKeys := make([]string, 0, 2)
	for _, column := range columns {
		if column.PK > 0 {
			primaryKeys = append(primaryKeys, column.Name)
		}
	}
	sort.Strings(primaryKeys)
	return primaryKeys, nil
}

func rebuildUpstreamProxyCountryRules(tx *gorm.DB) error {
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
