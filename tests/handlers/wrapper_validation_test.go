package handlers_test

import (
	"testing"

	"cloudstackctl/pkg/handlers"
)

func TestWrapperValidationErrors(t *testing.T) {
	t.Run("get invalid json", func(t *testing.T) {
		if _, err := handlers.GetCloudStackResource([]byte("{")); err == nil {
			t.Fatal("expected invalid JSON error")
		}
	})

	t.Run("get unsupported kind", func(t *testing.T) {
		if _, err := handlers.GetCloudStackResource([]byte(`{"kind":"Nope"}`)); err == nil {
			t.Fatal("expected unsupported kind error")
		}
	})

	t.Run("describe missing name", func(t *testing.T) {
		if _, err := handlers.DescribeCloudStackResource([]byte(`{"kind":"Network"}`)); err == nil {
			t.Fatal("expected missing name error")
		}
	})

	t.Run("describe invalid json", func(t *testing.T) {
		if _, err := handlers.DescribeCloudStackResource([]byte("{")); err == nil {
			t.Fatal("expected invalid JSON error")
		}
	})

	t.Run("delete missing name", func(t *testing.T) {
		if _, err := handlers.DeleteCloudStackResource([]byte(`{"kind":"Network"}`)); err == nil {
			t.Fatal("expected missing name error")
		}
	})

	t.Run("delete invalid json", func(t *testing.T) {
		if _, err := handlers.DeleteCloudStackResource([]byte("{")); err == nil {
			t.Fatal("expected invalid JSON error")
		}
	})

	t.Run("apply invalid json", func(t *testing.T) {
		if _, err := handlers.ApplyCloudStackResource([]byte("{")); err == nil {
			t.Fatal("expected invalid JSON error")
		}
	})

	t.Run("apply unsupported kind", func(t *testing.T) {
		if _, err := handlers.ApplyCloudStackResource([]byte(`{"kind":"Nope"}`)); err == nil {
			t.Fatal("expected unsupported kind error")
		}
	})
}
