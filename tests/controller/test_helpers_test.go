package controller_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"testing"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func testDSN() string {
	if dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")); dsn != "" {
		return dsn
	}
	if dsn := strings.TrimSpace(os.Getenv("DATABASE_DSN")); dsn != "" {
		return dsn
	}
	return "host=localhost user=postgres password=secret dbname=cloudstackctl port=5432 sslmode=disable"
}

func dsnWithSearchPath(baseDSN, schema string) string {
	if strings.HasPrefix(baseDSN, "postgres://") || strings.HasPrefix(baseDSN, "postgresql://") {
		sep := "?"
		if strings.Contains(baseDSN, "?") {
			sep = "&"
		}
		return baseDSN + sep + "search_path=" + schema
	}
	return baseDSN + " search_path=" + schema
}

func sanitizeSchemaName(s string) string {
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	out := strings.ToLower(s)
	out = strings.ReplaceAll(out, " ", "_")
	out = re.ReplaceAllString(out, "_")
	out = strings.Trim(out, "_")
	if out == "" {
		out = "test"
	}
	return out
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	baseDSN := testDSN()
	rootDB, err := gorm.Open(postgres.Open(baseDSN), &gorm.Config{})
	if err != nil {
		// If no explicit test DSN was provided, avoid hard-failing developers
		// that do not have a local PostgreSQL instance running.
		if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" && strings.TrimSpace(os.Getenv("DATABASE_DSN")) == "" {
			t.Skipf("skipping PostgreSQL-backed controller tests: %v", err)
		}
		t.Fatalf("failed to connect to test postgres: %v", err)
	}

	schema := sanitizeSchemaName(fmt.Sprintf("%s_%d", t.Name(), time.Now().UnixNano()))
	if err := rootDB.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)).Error; err != nil {
		t.Fatalf("failed to create test schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_ = rootDB.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema)).Error
	})

	tdb, err := gorm.Open(postgres.Open(dsnWithSearchPath(baseDSN, schema)), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect test schema %s: %v", schema, err)
	}
	if err := tdb.AutoMigrate(&v1.Application{}, &v1.Component{}, &v1.VirtualMachineSpecResource{}, &v1.VirtualMachine{}); err != nil {
		t.Fatalf("failed to migrate postgres test schema %s: %v", schema, err)
	}
	db.DB = tdb
	return tdb
}
