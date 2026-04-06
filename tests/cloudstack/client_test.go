package cloudstack_test

import (
	"os"
	"path/filepath"
	"testing"

	cloudstackpkg "cloudstackctl/pkg/cloudstack"
)

func resetEnvForClient(t *testing.T) {
	t.Helper()
	t.Setenv("CLOUDSTACK_ENDPOINT", "")
	t.Setenv("CLOUDSTACK_API_KEY", "")
	t.Setenv("CLOUDSTACK_SECRET_KEY", "")
	t.Setenv("VERIFY_SSL", "")
}

func TestNewClient_MissingCredentials(t *testing.T) {
	resetEnvForClient(t)
	// Force NewClient to skip optional default .env.cloudstack discovery.
	cloudstackpkg.SetConfigFile(filepath.Join(t.TempDir(), "does-not-exist.env"))
	t.Cleanup(func() { cloudstackpkg.SetConfigFile("") })

	client, err := cloudstackpkg.NewClient()
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if client != nil {
		t.Fatal("expected nil client when credentials are missing")
	}
}

func TestNewClient_ConfigFileSuccess(t *testing.T) {
	resetEnvForClient(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cloudstack.env")
	content := "CLOUDSTACK_ENDPOINT=http://example.invalid/client/api\n" +
		"CLOUDSTACK_API_KEY=test-key\n" +
		"CLOUDSTACK_SECRET_KEY=test-secret\n" +
		"VERIFY_SSL=false\n"
	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		t.Fatalf("failed writing config file: %v", err)
	}
	cloudstackpkg.SetConfigFile(cfg)
	t.Cleanup(func() { cloudstackpkg.SetConfigFile("") })

	client, err := cloudstackpkg.NewClient()
	if err != nil {
		t.Fatalf("expected client from config file, got error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
