package token

import (
	"testing"
	"time"
)

func TestGenerateReturnsNonEmpty(t *testing.T) {
	gen := NewGenerator()
	tok, err := gen.Generate("user1", "room-1", time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if tok == "" {
		t.Error("token should not be empty")
	}
}

func TestGenerateReturnsHexEncoded(t *testing.T) {
	gen := NewGenerator()
	tok, err := gen.Generate("user1", "room-1", time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// 32 bytes = 64 hex chars
	if len(tok) != 64 {
		t.Errorf("token length = %d, want 64", len(tok))
	}
}

func TestGenerateReturnsDifferentTokens(t *testing.T) {
	gen := NewGenerator()
	tok1, _ := gen.Generate("user1", "room-1", time.Now().Add(5*time.Minute))
	tok2, _ := gen.Generate("user1", "room-1", time.Now().Add(5*time.Minute))
	if tok1 == tok2 {
		t.Error("two generated tokens should not be equal")
	}
}
