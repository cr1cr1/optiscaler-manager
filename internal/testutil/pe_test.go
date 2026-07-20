package testutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

func TestStringInfoPE_FileVersionRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		pe32plus bool
		strs     map[string]string
		quad     [4]uint16
		want     string
	}{
		{"PE32 product version wins over placeholder fixed", false,
			map[string]string{"ProductVersion": "0.9.4"}, [4]uint16{1, 0, 0, 0}, "0.9.4"},
		{"PE32+ fixed quad only", true, nil, [4]uint16{0, 7, 2, 0}, "0.7.2.0"},
		{"fixed quad wins when product is placeholder", false,
			map[string]string{"ProductVersion": "1.0.0.0"}, [4]uint16{2, 4, 12, 0}, "2.4.12.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "test.dll")
			data := testutil.StringInfoPE(tc.pe32plus, tc.strs, tc.quad)
			if err := os.WriteFile(p, data, 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := pever.FileVersion(p)
			if err != nil {
				t.Fatalf("FileVersion: %v", err)
			}
			if got != tc.want {
				t.Errorf("FileVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStringInfoPE_NoVersionWhenQuadZeroAndNoProductVersion(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.dll")
	data := testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{})
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := pever.FileVersion(p); err == nil {
		t.Error("want error for PE without version evidence")
	}
}

func TestStringInfoPE_DetectOptiScalerRoundTrip(t *testing.T) {
	t.Run("detected via OriginalFilename with no version evidence", func(t *testing.T) {
		dir := t.TempDir()
		dll := testutil.StringInfoPE(true,
			map[string]string{"OriginalFilename": "OptiScaler.dll"}, [4]uint16{})
		if err := os.WriteFile(filepath.Join(dir, "dxgi.dll"), dll, 0o644); err != nil {
			t.Fatal(err)
		}
		found, version := pever.DetectOptiScaler(dir)
		if !found {
			t.Error("DetectOptiScaler: want found")
		}
		if version != "" {
			t.Errorf("DetectOptiScaler version = %q, want %q", version, "")
		}
	})

	t.Run("detected with PE version from fixed quad", func(t *testing.T) {
		dir := t.TempDir()
		dll := testutil.StringInfoPE(false,
			map[string]string{"ProductName": "OptiScaler"}, [4]uint16{0, 9, 4, 0})
		if err := os.WriteFile(filepath.Join(dir, "OptiScaler.dll"), dll, 0o644); err != nil {
			t.Fatal(err)
		}
		found, version := pever.DetectOptiScaler(dir)
		if !found {
			t.Error("DetectOptiScaler: want found")
		}
		if version != "0.9.4.0" {
			t.Errorf("DetectOptiScaler version = %q, want %q", version, "0.9.4.0")
		}
	})
}
