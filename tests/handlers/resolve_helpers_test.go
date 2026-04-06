package handlers_test

import (
	"testing"

	"cloudstackctl/pkg/handlers"
)

func TestIsUUID(t *testing.T) {
	valid := []string{
		"123e4567-e89b-12d3-a456-426614174000",
		"123E4567-E89B-12D3-A456-426614174000",
	}
	invalid := []string{
		"",
		"not-a-uuid",
		"123e4567e89b12d3a456426614174000",
		"123e4567-e89b-12d3-a456-42661417400",
	}

	for _, s := range valid {
		if !handlers.IsUUID(s) {
			t.Fatalf("expected %q to be valid UUID", s)
		}
	}
	for _, s := range invalid {
		if handlers.IsUUID(s) {
			t.Fatalf("expected %q to be invalid UUID", s)
		}
	}
}
