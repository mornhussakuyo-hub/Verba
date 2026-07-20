package diagnostic

import (
	"slices"
	"testing"
)

func TestSortUsesDiagnosticContentAsTieBreakers(t *testing.T) {
	items := []Diagnostic{
		{Severity: Warning, Code: "VRB0002", File: "main.vrb", Line: 3, Column: 5, Message: "second"},
		{Severity: Error, Code: "VRB0001", File: "main.vrb", Line: 3, Column: 5, Message: "first", Hint: "fix it"},
		{Severity: Error, Code: "VRB0001", File: "main.vrb", Line: 3, Column: 5, Message: "first"},
	}

	Sort(items)

	want := []Diagnostic{
		{Severity: Error, Code: "VRB0001", File: "main.vrb", Line: 3, Column: 5, Message: "first"},
		{Severity: Error, Code: "VRB0001", File: "main.vrb", Line: 3, Column: 5, Message: "first", Hint: "fix it"},
		{Severity: Warning, Code: "VRB0002", File: "main.vrb", Line: 3, Column: 5, Message: "second"},
	}
	if !slices.Equal(items, want) {
		t.Fatalf("Sort() = %#v, want %#v", items, want)
	}
}
