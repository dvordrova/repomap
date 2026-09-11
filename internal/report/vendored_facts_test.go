package report

import "testing"

// Facts observed inside vendored or generated dependency trees describe the
// dependency's authors' work; the reader's lists keep the repository's own.
func TestVendoredAndCatalogPathsStayOutOfReaderLists(t *testing.T) {
	for path, want := range map[string]bool{
		"vendor/github.com/BurntSushi/toml/decode.go": true,
		"front/node_modules/parcel/index.js":          true,
		"deploy/.terraform/providers/x/README.md":     true,
		"main.go":                            false,
		"internal/vendorlike/vendor.go":      false,
		"database/migrations/00001_init.sql": false,
	} {
		if got := vendoredPath(path); got != want {
			t.Fatalf("vendoredPath(%q) = %v, want %v", path, got, want)
		}
	}
	for name, want := range map[string]bool{"pg_type": true, "PG_ATTRIBUTE": true, "information_schema.columns": true, "events": false, "pgboss.jobs": false} {
		if got := catalogTable(name); got != want {
			t.Fatalf("catalogTable(%q) = %v, want %v", name, got, want)
		}
	}
}
