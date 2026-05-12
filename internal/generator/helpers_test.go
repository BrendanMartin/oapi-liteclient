package generator

import (
	"testing"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

func TestTagToFilename(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"CAPAs", "capas"},
		{"Chart Of Accounts", "chart_of_accounts"},
		{"Pets", "pets"},
		{"Users", "users"},
		{"", "general"},
		{"my-tag", "my_tag"},
		{"DocumentTypes", "document_types"},
		{"XMLParser", "xmlparser"},
		{"Shipment Line Items (V3)", "shipment_line_items_v3"},
	}
	for _, tt := range tests {
		got := tagToFilename(tt.in)
		if got != tt.want {
			t.Errorf("tagToFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTagToClassName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"CAPAs", "CAPAs"},
		{"Chart Of Accounts", "ChartOfAccounts"},
		{"Pets", "Pets"},
		{"", "General"},
		{"my-tag", "MyTag"},
		{"document_types", "DocumentTypes"},
		{"Shipment Line Items (V3)", "ShipmentLineItemsV3"},
	}
	for _, tt := range tests {
		got := tagToClassName(tt.in)
		if got != tt.want {
			t.Errorf("tagToClassName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGroupEndpointsByTag(t *testing.T) {
	endpoints := []ir.Endpoint{
		{OperationID: "listPets", Tags: []string{"Pets"}},
		{OperationID: "createPet", Tags: []string{"Pets"}},
		{OperationID: "getUser", Tags: []string{"Users"}},
		{OperationID: "ping"},
	}

	groups, hasTags := groupEndpointsByTag(endpoints)
	if !hasTags {
		t.Fatal("expected hasTags=true")
	}
	if len(groups["Pets"]) != 2 {
		t.Errorf("expected 2 Pets endpoints, got %d", len(groups["Pets"]))
	}
	if len(groups["Users"]) != 1 {
		t.Errorf("expected 1 Users endpoint, got %d", len(groups["Users"]))
	}
	if len(groups[""]) != 1 {
		t.Errorf("expected 1 untagged endpoint, got %d", len(groups[""]))
	}
}

func TestGroupEndpointsByTagNoTags(t *testing.T) {
	endpoints := []ir.Endpoint{
		{OperationID: "ping"},
		{OperationID: "health"},
	}

	_, hasTags := groupEndpointsByTag(endpoints)
	if hasTags {
		t.Fatal("expected hasTags=false when no endpoints have tags")
	}
}

func TestValidateTagFilenames(t *testing.T) {
	t.Run("no collision", func(t *testing.T) {
		groups := map[string][]ir.Endpoint{
			"Pets":  {{OperationID: "listPets"}},
			"Users": {{OperationID: "getUser"}},
		}
		if err := validateTagFilenames(groups); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("collision", func(t *testing.T) {
		groups := map[string][]ir.Endpoint{
			"my_pets": {{OperationID: "a"}},
			"my-pets": {{OperationID: "b"}},
		}
		if err := validateTagFilenames(groups); err == nil {
			t.Error("expected collision error")
		}
	})

	t.Run("reserved name", func(t *testing.T) {
		groups := map[string][]ir.Endpoint{
			"client": {{OperationID: "a"}},
		}
		if err := validateTagFilenames(groups); err == nil {
			t.Error("expected reserved name error")
		}
	})
}
