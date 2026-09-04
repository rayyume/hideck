package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/yibaiba/hideck/internal/phonelookup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidPhoneContact = errors.New("invalid phone contact")

type PhoneContact struct {
	Number    string    `gorm:"column:number;primaryKey" json:"number"`
	Name      string    `gorm:"column:name;not null" json:"name"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

type PhoneContactInput struct {
	Number string
	Name   string
	Region string
}

func (PhoneContact) TableName() string { return "phone_contacts" }

func NormalizePhoneContactNumber(raw string) string {
	return NormalizePhoneContactNumberWithRegion(raw, "")
}

func NormalizePhoneContactNumberWithRegion(raw, region string) string {
	return phonelookup.CanonicalWithRegion(raw, region)
}

func ListPhoneContacts(ctx context.Context) ([]PhoneContact, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var rows []PhoneContact
	err := DB.WithContext(ctx).Order("updated_at DESC").Find(&rows).Error
	return rows, err
}

func UpsertPhoneContact(ctx context.Context, number, name string) (PhoneContact, error) {
	return UpsertPhoneContactWithRegion(ctx, PhoneContactInput{Number: number, Name: name})
}

func UpsertPhoneContactWithRegion(ctx context.Context, input PhoneContactInput) (PhoneContact, error) {
	number := NormalizePhoneContactNumberWithRegion(input.Number, input.Region)
	name := strings.TrimSpace(input.Name)
	if number == "" || name == "" {
		return PhoneContact{}, ErrInvalidPhoneContact
	}
	if DB == nil {
		return PhoneContact{}, gorm.ErrInvalidDB
	}
	row := PhoneContact{Number: number, Name: name, UpdatedAt: time.Now()}
	err := DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "number"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return PhoneContact{}, err
	}
	if err := DB.WithContext(ctx).Where("number = ?", number).First(&row).Error; err != nil {
		return PhoneContact{}, err
	}
	return row, nil
}

func GetPhoneContact(ctx context.Context, number string) (PhoneContact, error) {
	number = NormalizePhoneContactNumber(number)
	if number == "" || DB == nil {
		return PhoneContact{}, gorm.ErrRecordNotFound
	}
	var row PhoneContact
	err := DB.WithContext(ctx).Where("number = ?", number).First(&row).Error
	return row, err
}

func DeletePhoneContact(ctx context.Context, number string) error {
	return DeletePhoneContactWithRegion(ctx, number, "")
}

func DeletePhoneContactWithRegion(ctx context.Context, number, region string) error {
	number = NormalizePhoneContactNumberWithRegion(number, region)
	if number == "" {
		return ErrInvalidPhoneContact
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	return DB.WithContext(ctx).Where("number = ?", number).Delete(&PhoneContact{}).Error
}

func LookupPhoneIdentity(ctx context.Context, raw string) phonelookup.Result {
	return LookupPhoneIdentityWithRegion(ctx, raw, "")
}

func LookupPhoneIdentityWithRegion(ctx context.Context, raw, region string) phonelookup.Result {
	result := phonelookup.LookupWithRegion(raw, region)
	canonical := phonelookup.CanonicalWithRegion(raw, region)
	if result.Number == "" && canonical == "" {
		return result
	}
	row, err := GetPhoneContact(ctx, canonical)
	if err != nil || strings.TrimSpace(row.Name) == "" {
		return result
	}
	return result.WithName(row.Name)
}
