package handlers_test

import (
	"testing"

	"cloudstackctl/pkg/handlers"
)

type projectOnlyParamsStub struct {
	projectID string
}

func (p *projectOnlyParamsStub) SetProjectid(v string) {
	p.projectID = v
}

type projectAndListAllParamsStub struct {
	projectID string
	listAll   bool
}

func (p *projectAndListAllParamsStub) SetProjectid(v string) {
	p.projectID = v
}

func (p *projectAndListAllParamsStub) SetListall(v bool) {
	p.listAll = v
}

func TestSetListAllOnParams_WithAllProjects(t *testing.T) {
	p := &projectAndListAllParamsStub{}
	handlers.SetListAllOnParams(p, true)

	if p.projectID != "-1" {
		t.Fatalf("expected project id -1, got %q", p.projectID)
	}
	if !p.listAll {
		t.Fatal("expected listAll=true when params support SetListall")
	}
}

func TestSetListAllOnParams_WithoutAllProjects(t *testing.T) {
	p := &projectAndListAllParamsStub{}
	handlers.SetListAllOnParams(p, false)

	if p.projectID != "" {
		t.Fatalf("expected empty project id when allProjects=false, got %q", p.projectID)
	}
	if p.listAll {
		t.Fatal("expected listAll=false when allProjects=false")
	}
}

func TestSetListAllOnParams_ProjectOnlyParams(t *testing.T) {
	p := &projectOnlyParamsStub{}
	handlers.SetListAllOnParams(p, true)

	if p.projectID != "-1" {
		t.Fatalf("expected project id -1, got %q", p.projectID)
	}
}
