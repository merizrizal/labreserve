package resources

import (
	"errors"
	"strings"
	"testing"
)

func TestCanonicalizeCreateUsesUnicodeEdgeTrimmingAndPreservesContent(t *testing.T) {
	input := CreateRequest{
		Code:        "\u2003MiX/Code\t x\u00a0",
		Name:        "\u00a0Lab\tReserve\n  west\u2003",
		Description: "\u2003Cafe\u0301  \tline\nnext\u00a0",
		Active:      false,
	}

	got, err := canonicalizeCreate(input)
	if err != nil {
		t.Fatalf("canonicalize create: %v", err)
	}
	if got.Code != "MiX/Code\t x" || got.Name != "Lab\tReserve\n  west" ||
		got.Description != "Cafe\u0301  \tline\nnext" || got.Active {
		t.Fatalf("canonical create values = %#v", got)
	}
	if got.Description == "Café  \tline\nnext" {
		t.Fatal("canonicalization unexpectedly normalized Unicode composition")
	}
}

func TestCanonicalizeCreateAllowsEmptyDescriptionAndLongText(t *testing.T) {
	for _, description := range []string{"", " \t\n", "\u2003\u00a0"} {
		got, err := canonicalizeCreate(CreateRequest{
			Code: "arbitrary-code", Name: "Name", Description: description, Active: true,
		})
		if err != nil {
			t.Fatalf("canonicalize empty description %q: %v", description, err)
		}
		if got.Description != "" || !got.Active {
			t.Fatalf("canonicalized empty description/state = %#v", got)
		}
	}

	long := strings.Repeat("x", 100_000)
	got, err := canonicalizeCreate(CreateRequest{Code: long, Name: long, Description: long})
	if err != nil {
		t.Fatalf("canonicalize long valid text: %v", err)
	}
	if got.Code != long || got.Name != long || got.Description != long {
		t.Fatal("canonicalization changed long valid text")
	}
}

func TestCanonicalizeCreateRejectsEmptyRequiredTextAndInvalidUTF8(t *testing.T) {
	tests := []struct {
		name  string
		input CreateRequest
	}{
		{name: "empty code", input: CreateRequest{Name: "Name"}},
		{name: "unicode whitespace code", input: CreateRequest{Code: "\u2003\u00a0", Name: "Name"}},
		{name: "empty name", input: CreateRequest{Code: "Code"}},
		{name: "unicode whitespace name", input: CreateRequest{Code: "Code", Name: "\u2003\u00a0"}},
		{name: "invalid code encoding", input: CreateRequest{Code: string([]byte{0xff}), Name: "Name"}},
		{name: "invalid name encoding", input: CreateRequest{Code: "Code", Name: string([]byte{0xff})}},
		{name: "invalid optional description encoding", input: CreateRequest{Code: "Code", Name: "Name", Description: string([]byte{0xff})}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := canonicalizeCreate(test.input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("canonicalize create error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestCanonicalizeEditKeepsImmutableCodeAssertionAndExactText(t *testing.T) {
	got, err := canonicalizeEdit(EditRequest{
		ResourceID:  "resource-id",
		Code:        "\u2003MiXeD-Code\u00a0",
		Name:        "\u00a0A name\u00a0",
		Description: "\u2003Description e\u0301\u00a0",
	})
	if err != nil {
		t.Fatalf("canonicalize edit: %v", err)
	}
	if got.ResourceID != "resource-id" || got.Code != "MiXeD-Code" || got.Name != "A name" ||
		got.Description != "Description e\u0301" {
		t.Fatalf("canonical edit values = %#v", got)
	}
	if got.Code == strings.ToLower(got.Code) || got.Description == "Description é" {
		t.Fatal("canonicalization changed code casing or Unicode composition")
	}
}

func TestCanonicalizeEditAllowsClearingDescription(t *testing.T) {
	got, err := canonicalizeEdit(EditRequest{
		ResourceID:  "resource-id",
		Code:        "Code",
		Name:        "Name",
		Description: "\u2003\u00a0",
	})
	if err != nil {
		t.Fatalf("canonicalize edit that clears description: %v", err)
	}
	if got.Description != "" {
		t.Fatalf("cleared description = %q, want empty string", got.Description)
	}
}

func TestCanonicalizeEditAllowsLongValidText(t *testing.T) {
	long := strings.Repeat("x", 100_000)
	got, err := canonicalizeEdit(EditRequest{
		ResourceID: "resource-id", Code: long, Name: long, Description: long,
	})
	if err != nil {
		t.Fatalf("canonicalize long edit values: %v", err)
	}
	if got.Code != long || got.Name != long || got.Description != long {
		t.Fatal("canonicalization changed long edit values")
	}
}

func TestCanonicalizeEditRejectsInvalidRequiredAndOptionalText(t *testing.T) {
	tests := []struct {
		name  string
		input EditRequest
	}{
		{name: "missing code assertion", input: EditRequest{ResourceID: "id", Name: "Name"}},
		{name: "whitespace name", input: EditRequest{ResourceID: "id", Code: "Code", Name: "\u2003\u00a0"}},
		{name: "invalid description encoding", input: EditRequest{ResourceID: "id", Code: "Code", Name: "Name", Description: string([]byte{0xff})}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := canonicalizeEdit(test.input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("canonicalize edit error = %v, want ErrInvalidInput", err)
			}
		})
	}
}
