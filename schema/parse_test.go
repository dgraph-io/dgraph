package schema

import (
	"testing"
)

func TestSchema(t *testing.T) {
	err := Parse("test_schema")
	if err != nil {
		t.Error(err)
	}
}

func TestSchema1_Error(t *testing.T) {
	err := Parse("test_schema1")
	if err == nil {
		t.Error("Expected error")
	}
}

func TestSchema2_Error(t *testing.T) {
	err := Parse("test_schema2")
	if err == nil {
		t.Error("Expected error")
	}
}

func TestSchema3_Error(t *testing.T) {
	err := Parse("test_schema3")
	if err == nil {
		t.Error("Expected error")
	}
}
