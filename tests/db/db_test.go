package db_test

import (
	"testing"

	dbpkg "cloudstackctl/db"
)

func TestInit_InvalidDSNReturnsError(t *testing.T) {
	// Point to an unreachable port with short timeout so the test is deterministic.
	t.Setenv("DATABASE_DSN", "host=127.0.0.1 user=postgres password=secret dbname=cloudstackctl port=1 sslmode=disable connect_timeout=1")
	err := dbpkg.Init()
	if err == nil {
		t.Fatal("expected db.Init to fail with invalid/unreachable DSN")
	}
}
