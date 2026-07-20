package region

import (
	"bytes"
	"testing"

	"github.com/verba-lang/verba/internal/source"
)

func TestScanPreservesRawIslandBytes(t *testing.T) {
	content := []byte("module example\r\nembed json data until done\r\n{\"end\": \"done\"}\r\nend done\r\ndone\r\nroute x\r\n")
	file, diagnostics := source.New("example.vrb", content)
	if len(diagnostics) != 0 {
		t.Fatalf("source diagnostics = %#v", diagnostics)
	}
	result := Scan(file)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Scan() diagnostics = %#v", result.Diagnostics)
	}
	island, ok := result.IslandAtHeader(2)
	if !ok || !island.Terminated || island.TerminatorLine != 5 {
		t.Fatalf("Scan() island = %#v", island)
	}
	want := []byte("{\"end\": \"done\"}\r\nend done")
	if raw := file.Slice(island.Content.Start, island.Content.End); !bytes.Equal(raw, want) {
		t.Fatalf("island raw = %q, want %q", raw, want)
	}
}

func TestScanRequiresExactTerminatorLine(t *testing.T) {
	file, _ := source.New("example.vrb", []byte("module example\nembed text data until done\nvalue\n    done\n"))
	result := Scan(file)
	island, ok := result.IslandAtHeader(2)
	if !ok || island.Terminated || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "VRB0242" {
		t.Fatalf("Scan() = %#v", result)
	}
}
