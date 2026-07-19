package profile

import (
	"bytes"
	"strings"
	"testing"
)

func TestDefaultINISafeDefaults(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDefaultINI(&buf); err != nil {
		t.Fatalf("WriteDefaultINI: %v", err)
	}
	out := buf.String()
	t.Logf("curated ini:\n%s", out)

	for _, want := range []string{
		"[Upscalers]",
		"Dx11Upscaler=auto",
		"Dx12Upscaler=auto",
		"VulkanUpscaler=auto",
		"[FrameGen]",
		"FGType=auto",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("curated ini missing %q", want)
		}
	}

	// Deterministic: two writes produce identical bytes.
	var buf2 bytes.Buffer
	if err := WriteDefaultINI(&buf2); err != nil {
		t.Fatalf("WriteDefaultINI: %v", err)
	}
	if buf.String() != buf2.String() {
		t.Error("WriteDefaultINI is not deterministic")
	}
}
