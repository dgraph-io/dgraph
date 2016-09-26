package schema

import "testing"

func TestParse(t *testing.T) {
	err := Parse("test_schema")

	if err != nil {
		t.Error(err)
	}

}
