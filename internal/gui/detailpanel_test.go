package gui

import "testing"

// TestDetailPanelWidth_Clamps: the detail panel tracks 30% of the window
// width, clamped to [300, 480] so narrow windows keep a usable panel and
// ultrawide windows do not grow an absurd sidebar.
func TestDetailPanelWidth_Clamps(t *testing.T) {
	tests := []struct {
		windowW float32
		want    float32
	}{
		{800, 300},  // 240 clamps up to the floor
		{1100, 330}, // proportional inside the band
		{2000, 480}, // 600 clamps down to the ceiling
	}
	for _, tt := range tests {
		if got := detailPanelWidth(tt.windowW); got != tt.want {
			t.Errorf("detailPanelWidth(%v) = %v, want %v", tt.windowW, got, tt.want)
		}
	}
}
