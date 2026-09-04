package db

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type phoneContactBatchOptions struct {
	Region      string
	SkipInvalid bool
}

func upsertPhoneContactBatch(
	ctx context.Context,
	inputs []PhoneContactInput,
	options phoneContactBatchOptions,
) ([]PhoneContact, int, error) {
	if DB == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	rows := make([]PhoneContact, 0, len(inputs))
	skipped := 0
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		groupIDs := make(map[string]string)
		for _, input := range inputs {
			if strings.TrimSpace(input.Region) == "" {
				input.Region = options.Region
			}
			if err := assignImportContactID(&input, groupIDs); err != nil {
				return err
			}
			row, err := upsertPhoneContactWithDB(ctx, tx, input)
			if err == nil {
				rows = append(rows, row)
				continue
			}
			if options.SkipInvalid && errors.Is(err, ErrInvalidPhoneContact) {
				skipped++
				continue
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, skipped, nil
}
