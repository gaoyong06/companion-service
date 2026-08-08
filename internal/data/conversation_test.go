package data

import (
	"testing"

	"companion-service/internal/conf"
)

func TestNewStoreRequiresDatabaseSource(t *testing.T) {
	_, err := resolveDatabaseSource(&conf.Data{Database: &conf.Database{SourceEnv: "COMPANION_TEST_MISSING_DSN"}})
	if err == nil {
		t.Fatal("expected missing database source error")
	}
}

func TestNewStoreReadsDatabaseSourceFromEnvironment(t *testing.T) {
	const expected = "root:root@tcp(127.0.0.1:1)/companion-service?parseTime=true"
	t.Setenv("COMPANION_TEST_DSN", expected)
	source, err := resolveDatabaseSource(&conf.Data{Database: &conf.Database{SourceEnv: "COMPANION_TEST_DSN"}})
	if err != nil {
		t.Fatalf("resolve database source: %v", err)
	}
	if source != expected {
		t.Fatalf("unexpected database source: %s", source)
	}
}
