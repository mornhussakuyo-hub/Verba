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

func TestSourcePreservesIslandBytes(t *testing.T) {
	input := []byte("module   demo\r\n\r\nembed text message until done\r\nfirst  \r\nsecond\r\ndone\r\n")
	want := []byte("module demo\n\nembed text message until done\nfirst  \r\nsecond\ndone\n")
	formatted := Source(input)
	if !bytes.Equal(formatted, want) {
		t.Fatalf("Source() = %q, want %q", formatted, want)
	}
}

func TestSourcePreservesControlledLiteralWhitespace(t *testing.T) {
	input := []byte("module demo\nfunction message\noutput string\nbegin\n  return   text   hello,  world!  \nend\n")
	want := []byte("module demo\nfunction message\noutput string\nbegin\n    return text hello,  world!  \nend\n")
	formatted := Source(input)
	if !bytes.Equal(formatted, want) {
		t.Fatalf("Source() = %q, want %q", formatted, want)
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
