package pickdir

import (
	"context"
	"errors"
	"testing"
)

func TestPickUnavailableWhenNoTools(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := Pick(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	t.Log("unavailable path errors cleanly without picker tools")
}
