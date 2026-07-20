package lexer

import (
	"testing"

	"github.com/verba-lang/verba/internal/region"
	"github.com/verba-lang/verba/internal/source"
)

func TestLexSeparatesControlledLiteralsAndIslands(t *testing.T) {
	content := []byte("module example\nlet endpoint to be url https://example.com/a?q=1\nrespond text 200 hello, world!\nembed json value until done\n{\"punctuation\": [1, 2]}\ndone\n")
	file, _ := source.New("example.vrb", content)
	regions := region.Scan(file)
	result := Lex(file, regions)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", result.Diagnostics)
	}
	var literals []string
	for _, token := range result.Tokens {
		if token.Kind == ControlledLiteral {
			literals = append(literals, token.Text)
		}
		if token.Text == `{"punctuation":` {
			t.Fatalf("island content leaked into core tokens: %#v", result.Tokens)
		}
	}
	if len(literals) != 2 || literals[0] != "https://example.com/a?q=1" || literals[1] != "hello, world!" {
		t.Fatalf("controlled literals = %#v", literals)
	}
}

func TestLexReportsInvalidCoreTokenAndNumber(t *testing.T) {
	file, _ := source.New("example.vrb", []byte("module bad-name\nlet value to be 1.2.3\n"))
	result := Lex(file, region.Scan(file))
	if len(result.Diagnostics) != 2 || result.Diagnostics[0].Code != "VRB0601" || result.Diagnostics[1].Code != "VRB0602" {
		t.Fatalf("Lex() diagnostics = %#v", result.Diagnostics)
	}
}

func TestSplitControlledPreservesPayload(t *testing.T) {
	prefix, payload, kind, ok := SplitControlled("return   text   hello,  world!  ")
	if !ok || kind != ControlledLiteral || prefix != "return   text" || payload != "hello,  world!  " {
		t.Fatalf("SplitControlled() = %q, %q, %v, %v", prefix, payload, kind, ok)
	}
}
