package gui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

func TestChunkRows(t *testing.T) {
	rows := make([]ui.GameRow, 7)
	for i := range rows {
		rows[i].Title = string(rune('A' + i))
	}
	chunks := chunkRows(rows, 3)
	if len(chunks) != 3 || len(chunks[0]) != 3 || len(chunks[1]) != 3 || len(chunks[2]) != 1 {
		t.Fatalf("chunkRows(7,3) = %v", lens(chunks))
	}
	if got := chunkRows(rows, 1); len(got) != 7 {
		t.Fatalf("chunkRows(7,1) = %d chunks", len(got))
	}
	if got := chunkRows(rows, 0); len(got) != 7 {
		t.Fatalf("chunkRows(7,0) must clamp to 1 col, got %d chunks", len(got))
	}
	if got := chunkRows(nil, 4); len(got) != 0 {
		t.Fatalf("chunkRows(nil) = %d chunks", len(got))
	}
	t.Logf("chunks: %v", lens(chunks))
}

func lens(chunks [][]ui.GameRow) []int {
	var out []int
	for _, c := range chunks {
		out = append(out, len(c))
	}
	return out
}

func TestQuickInstallButtonLabelByStatus(t *testing.T) {
	clean := &ui.GameRow{Title: "A"}
	if got := quickLabel(clean); got != "Install" {
		t.Errorf("clean row: %q", got)
	}
	installed := &ui.GameRow{Title: "B", Status: domain.StatusCommitted}
	if got := quickLabel(installed); got != "Uninstall" {
		t.Errorf("installed row: %q", got)
	}
	failed := &ui.GameRow{Title: "C", Status: domain.StatusFailed, Actionable: true}
	if got := quickLabel(failed); got != "Install" {
		t.Errorf("failed row: %q (retry counts as install)", got)
	}
}

func TestGridSmoke(t *testing.T) {
	cover := filepath.Join(t.TempDir(), "cover.png")
	img := image.NewRGBA(image.Rect(0, 0, 8, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{120, 40, 40, 255})
		}
	}
	f, err := os.Create(cover)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	m := newModel(Config{})
	m.state = ui.State{
		StatusLine: "3 games",
		Mode:       ui.ViewGrid,
		Rows: []ui.GameRow{
			{Title: "Cyberpunk 2077", AppID: "1091500", Status: domain.StatusCommitted,
				Platform: "Steam", CoverPath: cover,
				TechBadges: []ui.Badge{{Label: "DLSS", Tone: ui.ToneGreen}, {Label: "FSR", Tone: ui.ToneRed}}},
			{Title: "Beat Saber", AppID: "620980", Platform: "Steam", CoverPath: cover},
			{Title: "Starfield", AppID: "1716740", Platform: "Steam", CoverPath: cover, Status: domain.StatusFailed, Actionable: true},
		},
	}

	out := filepath.Join(t.TempDir(), "grid.png")
	if err := renderToPNG(out, 1000, 700, m.rootView); err != nil {
		t.Fatalf("renderToPNG grid: %v", err)
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		t.Fatalf("empty grid frame: %v", err)
	}
	t.Logf("grid frame: %d bytes", st.Size())
}

func TestGridToggleRendersListMode(t *testing.T) {
	m := newModel(Config{})
	m.state = ui.State{
		Mode: ui.ViewList,
		Rows: []ui.GameRow{{Title: "A", AppID: "1"}},
	}
	out := filepath.Join(t.TempDir(), "list.png")
	if err := renderToPNG(out, 1000, 700, m.rootView); err != nil {
		t.Fatalf("renderToPNG list mode: %v", err)
	}
	if st, _ := os.Stat(out); st == nil || st.Size() == 0 {
		t.Fatal("empty list frame")
	}
	t.Log("list mode renders after toggle")
}
