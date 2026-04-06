package yamlutil_test

import (
	"os"
	"path/filepath"
	"testing"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/yamlutil"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "resource.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp yaml: %v", err)
	}
	return path
}

func TestParseYAML_Application(t *testing.T) {
	path := writeTempYAML(t, `apiVersion: cloudstackctl/v1
kind: Application
metadata:
  name: demo
spec:
  components: []
`)

	obj, err := yamlutil.ParseYAML(path)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	app, ok := obj.(*v1.Application)
	if !ok {
		t.Fatalf("expected *v1.Application, got %T", obj)
	}
	if app.Metadata.Name != "demo" {
		t.Fatalf("unexpected application name: %q", app.Metadata.Name)
	}
}

func TestParseYAML_UnknownKind(t *testing.T) {
	path := writeTempYAML(t, `apiVersion: cloudstackctl/v1
kind: DoesNotExist
metadata:
  name: demo
`)

	_, err := yamlutil.ParseYAML(path)
	if err == nil {
		t.Fatal("expected error for unknown kind, got nil")
	}
}
