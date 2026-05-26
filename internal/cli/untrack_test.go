package cli

import "testing"

func TestChunkStrings(t *testing.T) {
	chunks := chunkStrings([]string{"a", "b", "c", "d", "e"}, 2)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks", len(chunks))
	}
	if got := chunks[0][0] + chunks[0][1] + chunks[2][0]; got != "abe" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	if chunks := chunkStrings([]string{""}, 100); len(chunks) != 0 {
		t.Fatalf("empty split should be omitted: %#v", chunks)
	}
}
