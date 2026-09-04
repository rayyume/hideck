package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/yibaiba/hideck/internal/phonelookup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidPhoneContact = errors.New("invalid phone contact")

const phoneContactIDPrefix = "contact_"
const phoneContactIDRandomBytes = 16

type PhoneContact struct {
	Number    string    `gorm:"column:number;primaryKey" json:"number"`
	ContactID string    `gorm:"column:contact_id;index" json:"contact_id"`
	Name      string    `gorm:"column:name;not null" json:"name"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

type PhoneContactInput struct {
	Number    string
	Name      string
	Region    string
	ContactID string
	GroupKey  string
}

type phoneContactIDRequest struct {
	Database  *gorm.DB
	Number    string
	Requested string
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
	err := phoneContactsQuery(ctx).Find(&rows).Error
	return rows, err
}

func ListPhoneContactsPage(ctx context.Context, limit, offset int) ([]PhoneContact, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var rows []PhoneContact
	err := phoneContactsQuery(ctx).Limit(limit).Offset(offset).Find(&rows).Error
	return rows, err
}

func CountPhoneContacts(ctx context.Context) (int64, error) {
	if DB == nil {
		return 0, gorm.ErrInvalidDB
	}
	var total int64
	err := DB.WithContext(ctx).Model(&PhoneContact{}).Count(&total).Error
	return total, err
}

func phoneContactsQuery(ctx context.Context) *gorm.DB {
	return DB.WithContext(ctx).Order("updated_at DESC, number ASC")
}

func UpsertPhoneContact(ctx context.Context, number, name string) (PhoneContact, error) {
	return UpsertPhoneContactWithRegion(ctx, PhoneContactInput{Number: number, Name: name})
}

func UpsertPhoneContactWithRegion(ctx context.Context, input PhoneContactInput) (PhoneContact, error) {
	if DB == nil {
		return PhoneContact{}, gorm.ErrInvalidDB
	}
	return upsertPhoneContactWithDB(ctx, DB, input)
}

func upsertPhoneContactWithDB(ctx context.Context, database *gorm.DB, input PhoneContactInput) (PhoneContact, error) {
	number := NormalizePhoneContactNumberWithRegion(input.Number, input.Region)
	name := strings.TrimSpace(input.Name)
	if number == "" || name == "" {
		return PhoneContact{}, ErrInvalidPhoneContact
	}
	contactID, err := resolvePhoneContactID(ctx, phoneContactIDRequest{
		Database: database, Number: number, Requested: input.ContactID,
	})
	if err != nil {
		return PhoneContact{}, err
	}
	row := PhoneContact{Number: number, ContactID: contactID, Name: name, UpdatedAt: time.Now()}
	err = database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "number"}},
		DoUpdates: clause.AssignmentColumns([]string{"contact_id", "name", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return PhoneContact{}, err
	}
	if err := database.WithContext(ctx).Where("number = ?", number).First(&row).Error; err != nil {
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
	_, err := DeletePhoneContactsWithRegion(ctx, []string{number}, region)
	return err
}

func UpsertPhoneContactsWithRegion(ctx context.Context, inputs []PhoneContactInput, region string) (imported, skipped int, err error) {
	if DB == nil {
		return 0, 0, gorm.ErrInvalidDB
	}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		groupIDs := make(map[string]string)
		for _, input := range inputs {
			if strings.TrimSpace(input.Region) == "" {
				input.Region = region
			}
			if err := assignImportContactID(&input, groupIDs); err != nil {
				return err
			}
			if _, upsertErr := upsertPhoneContactWithDB(ctx, tx, input); upsertErr != nil {
				if errors.Is(upsertErr, ErrInvalidPhoneContact) {
					skipped++
					continue
				}
				return upsertErr
			}
			imported++
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return imported, skipped, nil
}

func DeletePhoneContactsWithRegion(ctx context.Context, numbers []string, region string) (deleted int, err error) {
	if DB == nil {
		return 0, gorm.ErrInvalidDB
	}
	canonical := make([]string, 0, len(numbers))
	seen := map[string]struct{}{}
	for _, raw := range numbers {
		number := NormalizePhoneContactNumberWithRegion(raw, region)
		if number == "" {
			continue
		}
		if _, ok := seen[number]; ok {
			continue
		}
		seen[number] = struct{}{}
		canonical = append(canonical, number)
	}
	if len(canonical) == 0 {
		return 0, ErrInvalidPhoneContact
	}
	result := DB.WithContext(ctx).Where("number IN ?", canonical).Delete(&PhoneContact{})
	return int(result.RowsAffected), result.Error
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
	return result.WithContact(row.ContactID, row.Name)
}

func resolvePhoneContactID(ctx context.Context, request phoneContactIDRequest) (string, error) {
	if id := strings.TrimSpace(request.Requested); id != "" {
		if !validPhoneContactID(id) {
			return "", ErrInvalidPhoneContact
		}
		return id, nil
	}
	var existing PhoneContact
	err := request.Database.WithContext(ctx).
		Select("contact_id").Where("number = ?", request.Number).First(&existing).Error
	if err == nil && strings.TrimSpace(existing.ContactID) != "" {
		return existing.ContactID, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	return newPhoneContactID()
}

func assignImportContactID(input *PhoneContactInput, groups map[string]string) error {
	if input == nil || strings.TrimSpace(input.ContactID) != "" {
		return nil
	}
	key := strings.TrimSpace(input.GroupKey)
	if key == "" {
		return nil
	}
	if id := groups[key]; id != "" {
		input.ContactID = id
		return nil
	}
	id, err := newPhoneContactID()
	if err != nil {
		return err
	}
	groups[key] = id
	input.ContactID = id
	return nil
}

func newPhoneContactID() (string, error) {
	var raw [phoneContactIDRandomBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return phoneContactIDPrefix + hex.EncodeToString(raw[:]), nil
}

func validPhoneContactID(id string) bool {
	if len(id) != len(phoneContactIDPrefix)+(phoneContactIDRandomBytes*2) ||
		!strings.HasPrefix(id, phoneContactIDPrefix) {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, phoneContactIDPrefix))
	return err == nil
}
