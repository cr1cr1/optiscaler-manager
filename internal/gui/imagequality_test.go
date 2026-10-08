package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/anthonynsimon/bild/transform"
)

// Issue 042: covers rendered by the software path resample through
// shirei's scaled-image cache; the idle (steady-state) filter must be the
// highest-quality resampler available, not bilinear — the detail-pane
// poster visibly upscales otherwise.
func TestIdleImageFilterIsHighQuality(t *testing.T) {
	configureImageQuality()
	if ScaleIdleFilter.Support != transform.Lanczos.Support {
		t.Errorf("ScaleIdleFilter.Support = %v, want Lanczos (%v)", ScaleIdleFilter.Support, transform.Lanczos.Support)
	}
}
