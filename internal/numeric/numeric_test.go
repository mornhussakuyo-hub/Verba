package numeric

import "testing"

func TestCheckLiteralRanges(t *testing.T) {
	tests := []struct {
		value, target string
		valid         bool
	}{
		{"127", "int8", true},
		{"-128", "int8", true},
		{"128", "int8", false},
		{"-1", "uint8", false},
		{"255", "uint8", true},
		{"256", "uint8", false},
		{"18446744073709551615", "uint64", true},
		{"18446744073709551616", "uint64", false},
		{"9223372036854775808", "int", false},
		{"3.5", "int32", false},
		{"3", "float32", true},
		{"3.4028236e38", "float32", false},
		{"1.234567890123456789", "decimal", true},
	}
	for _, test := range tests {
		err := CheckLiteral(test.value, test.target)
		if (err == nil) != test.valid {
			t.Fatalf("CheckLiteral(%q, %q) error = %v, want valid %t", test.value, test.target, err, test.valid)
		}
	}
}

func TestClassify(t *testing.T) {
	for _, value := range []string{"0", "+12", "-9"} {
		if Classify(value) != Integer {
			t.Fatalf("Classify(%q) did not return Integer", value)
		}
	}
	for _, value := range []string{"1.25", "1e9", "-2.5e-3"} {
		if Classify(value) != Real {
			t.Fatalf("Classify(%q) did not return Real", value)
		}
	}
	for _, value := range []string{"1.", ".5", "1e", "word"} {
		if Classify(value) != Invalid {
			t.Fatalf("Classify(%q) did not return Invalid", value)
		}
	}
}
