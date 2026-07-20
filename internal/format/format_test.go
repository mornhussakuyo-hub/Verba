package format

import (
	"bytes"
	"os"
	"testing"
)

func TestSourceIsIdempotentAndPreservesIsland(t *testing.T) {
	input := []byte("module   demo\nroute x\nmethod get\npath   /x\nbegin\nrespond   text  200 ok\nend\n\nembed json value until done\n{\n  \"x\": 1\n}\ndone\n")
	once := Source(input)
	twice := Source(once)
	if !bytes.Equal(once, twice) {
		t.Fatalf("formatter is not idempotent:\n%s\n---\n%s", once, twice)
	}
	if !bytes.Contains(once, []byte("  \"x\": 1")) {
		t.Fatalf("island indentation changed:\n%s", once)
	}
}

func TestHelloExampleIsFormatted(t *testing.T) {
	source, err := os.ReadFile("../../examples/hello/main.vrb")
	if err != nil {
		t.Fatal(err)
	}
	formatted := Source(source)
	if !bytes.Equal(source, formatted) {
		index := firstDifference(source, formatted)
		t.Fatalf("example is not formatted; first difference at byte %d\nsource: %q\nformatted: %q", index, source[max(0, index-20):min(len(source), index+40)], formatted[max(0, index-20):min(len(formatted), index+40)])
	}
}

func firstDifference(left, right []byte) int {
	for i := 0; i < min(len(left), len(right)); i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return min(len(left), len(right))
}
