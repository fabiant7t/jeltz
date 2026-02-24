package rules

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsTraversal_DirectAndWrapped(t *testing.T) {
	if !IsTraversal(errTraversal) {
		t.Fatal("expected direct traversal sentinel to be recognized")
	}

	wrapped := fmt.Errorf("wrapped: %w", errTraversal)
	if !IsTraversal(wrapped) {
		t.Fatal("expected wrapped traversal sentinel to be recognized")
	}

	joined := errors.Join(errors.New("other"), wrapped)
	if !IsTraversal(joined) {
		t.Fatal("expected joined wrapped traversal sentinel to be recognized")
	}
}
