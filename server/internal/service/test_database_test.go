package service

import (
	"testing"

	"gorm.io/gorm"
)

// closeTestDatabase releases SQLite file handles before testing.TempDir cleanup.
// Windows does not permit removal of an open database file.
func closeTestDatabase(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get test database handle: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
}
