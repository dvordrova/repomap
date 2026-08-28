// Package migration contains one-shot data conversion jobs. These jobs are
// libraries for an external operations runner and are not application paths.
package migration

import (
	"context"

	vendor "geo.example/locator"
)

type AddressRow struct {
	Street  string
	City    string
	Postal  string
	Country string
}

func BackfillVerificationIDs(ctx context.Context, endpoint, key string, rows []AddressRow) ([]string, error) {
	client, err := vendor.New(endpoint, key, "migration-lenient")
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		result, err := client.Validate(ctx, vendor.Candidate{
			Street: row.Street, City: row.City, PostalCode: row.Postal, Country: row.Country,
		})
		if err != nil {
			return nil, err
		}
		ids = append(ids, result.ID)
	}
	return ids, nil
}
