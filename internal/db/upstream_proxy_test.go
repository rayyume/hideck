package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yibaiba/hideck/internal/upstreamproxy"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) {
	t.Helper()
	if err := Init(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("Init() error=%v", err)
	}
	loadCountryTableFixture(t)
}

func loadCountryTableFixture(t *testing.T) {
	t.Helper()
	cachePath := filepath.Join(t.TempDir(), "mcc-mnc-table.json")
	rows := `[{"mcc":"310","mnc":"260","iso":"us","country":"United States","country_code":"US","network":"T-Mobile"}]`
	if err := os.WriteFile(cachePath, []byte(rows), 0o644); err != nil {
		t.Fatalf("WriteFile() error=%v", err)
	}
	result := upstreamproxy.InitCountryTable(context.Background(), upstreamproxy.CountryTableOptions{
		CachePath: cachePath,
		SourceURL: "http://127.0.0.1:1/missing",
	})
	if result.Err != nil {
		t.Fatalf("InitCountryTable() error=%v", result.Err)
	}
}

func TestUpstreamProxyCountryRuleSelectsEnabledProxyByHomeMCC(t *testing.T) {
	openTestDB(t)
	now := time.Now()
	if err := UpsertUpstreamProxy(UpstreamProxy{ID: "proxy-us", Name: "US", Addr: "127.0.0.1:1080", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertUpstreamProxy() error=%v", err)
	}
	if err := UpsertUpstreamProxyCountryRule(UpstreamProxyCountryRule{CountryCode: " us ", UpstreamProxyID: "proxy-us", Enabled: true}); err != nil {
		t.Fatalf("UpsertUpstreamProxyCountryRule() error=%v", err)
	}
	proxy, country, err := GetHomeMCCUpstreamProxy("310")
	if err != nil {
		t.Fatalf("GetHomeMCCUpstreamProxy() error=%v", err)
	}
	if country != "US" || proxy == nil || proxy.ID != "proxy-us" {
		t.Fatalf("proxy=%+v country=%q, want proxy-us/US", proxy, country)
	}
}

func TestUpstreamProxyCountryRuleDirectWhenNoRuleOrDisabled(t *testing.T) {
	openTestDB(t)
	if err := UpsertUpstreamProxy(UpstreamProxy{ID: "proxy-us", Addr: "127.0.0.1:1080", Enabled: true}); err != nil {
		t.Fatalf("UpsertUpstreamProxy() error=%v", err)
	}
	proxy, country, err := GetHomeMCCUpstreamProxy("310")
	if err != nil || proxy != nil || country != "US" {
		t.Fatalf("no rule proxy=%+v country=%q err=%v, want nil/US/nil", proxy, country, err)
	}
	if err := UpsertUpstreamProxyCountryRule(UpstreamProxyCountryRule{CountryCode: "US", UpstreamProxyID: "proxy-us", Enabled: false}); err != nil {
		t.Fatalf("UpsertUpstreamProxyCountryRule() error=%v", err)
	}
	proxy, country, err = GetHomeMCCUpstreamProxy("310")
	if err != nil || proxy != nil || country != "US" {
		t.Fatalf("disabled rule proxy=%+v country=%q err=%v, want nil/US/nil", proxy, country, err)
	}
}

func TestUpstreamProxyCountryRuleDirectWhenUnknownMCCOrMissingProxy(t *testing.T) {
	openTestDB(t)
	proxy, country, err := GetHomeMCCUpstreamProxy("404")
	if err != nil || proxy != nil || country != "" {
		t.Fatalf("unknown mcc proxy=%+v country=%q err=%v, want nil/empty/nil", proxy, country, err)
	}
	if err := UpsertUpstreamProxyCountryRule(UpstreamProxyCountryRule{CountryCode: "US", UpstreamProxyID: "missing", Enabled: true}); err != nil {
		t.Fatalf("UpsertUpstreamProxyCountryRule() error=%v", err)
	}
	proxy, country, err = GetHomeMCCUpstreamProxy("310")
	if err != nil || proxy != nil || country != "US" {
		t.Fatalf("missing proxy proxy=%+v country=%q err=%v, want nil/US/nil", proxy, country, err)
	}
}

func TestUpstreamProxyCountryRuleAllowsMultipleNodes(t *testing.T) {
	openTestDB(t)
	now := time.Now()
	if err := UpsertUpstreamProxy(UpstreamProxy{ID: "uk-a", Addr: "127.0.0.1:1081", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertUpstreamProxy(UpstreamProxy{ID: "uk-b", Addr: "127.0.0.1:1082", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertUpstreamProxyCountryRule(UpstreamProxyCountryRule{CountryCode: "US", UpstreamProxyID: "uk-a", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertUpstreamProxyCountryRule(UpstreamProxyCountryRule{CountryCode: "US", UpstreamProxyID: "uk-b", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	proxies, country, err := GetHomeMCCUpstreamProxies("310")
	if err != nil || country != "US" || len(proxies) != 2 {
		t.Fatalf("pool=%+v country=%q err=%v, want 2 US nodes", proxies, country, err)
	}
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		got := PickUpstreamProxy(proxies)
		if got == nil {
			t.Fatal("pick returned nil")
		}
		seen[got.ID] = true
	}
	if !seen["uk-a"] || !seen["uk-b"] {
		t.Fatalf("random pick should hit both nodes, got %v", seen)
	}
}

func TestMigrateUpstreamProxyCountryRuleCompositePKPreservesRowsAndIsIdempotent(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	legacySchema := `CREATE TABLE upstream_proxy_country_rules (
		country_code text PRIMARY KEY, upstream_proxy_id text, enabled numeric, updated_at datetime)`
	if err := database.Exec(legacySchema).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO upstream_proxy_country_rules
		(country_code, upstream_proxy_id, enabled) VALUES ('GB', 'uk-a', 1)`).Error; err != nil {
		t.Fatal(err)
	}

	if err := MigrateUpstreamProxyCountryRuleCompositePK(database); err != nil {
		t.Fatal(err)
	}
	primaryKeys, err := upstreamProxyCountryRulePrimaryKeys(database)
	if err != nil || strings.Join(primaryKeys, ",") != "country_code,upstream_proxy_id" {
		t.Fatalf("primary keys=%v err=%v", primaryKeys, err)
	}
	var before int
	if err := database.Raw("PRAGMA schema_version").Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateUpstreamProxyCountryRuleCompositePK(database); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := database.Raw("PRAGMA schema_version").Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("idempotent migration changed schema version: before=%d after=%d", before, after)
	}
	var count int64
	if err := database.Model(&UpstreamProxyCountryRule{}).Where(
		"country_code = ? AND upstream_proxy_id = ?", "GB", "uk-a",
	).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("migrated row count=%d err=%v", count, err)
	}
}
