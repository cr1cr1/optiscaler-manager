package discovery

import (
	"strings"
	"testing"
)

const xmlInfoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Cool App</string>
	<key>CFBundleExecutable</key>
	<string>coolapp</string>
	<key>CFBundleVersion</key>
	<string>42</string>
</dict>
</plist>
`

func TestParseXMLPlist(t *testing.T) {
	t.Run("xml plist yields key values", func(t *testing.T) {
		kv, err := parseXMLPlist(strings.NewReader(xmlInfoPlist))
		if err != nil {
			t.Fatalf("parseXMLPlist: %v", err)
		}
		if kv["CFBundleName"] != "Cool App" || kv["CFBundleExecutable"] != "coolapp" {
			t.Fatalf("got %v", kv)
		}
		t.Logf("plist keys: %v", kv)
	})

	t.Run("binary plist rejected", func(t *testing.T) {
		binary := "bplist00\xd4\x01\x02\x03\x00\x00\x00\x00"
		if _, err := parseXMLPlist(strings.NewReader(binary)); err == nil {
			t.Fatal("expected error for binary plist, got nil")
		} else {
			t.Logf("binary plist error: %v", err)
		}
	})

	t.Run("malformed xml errors", func(t *testing.T) {
		if _, err := parseXMLPlist(strings.NewReader("<?xml version=\"1.0\"?><plist><dict><key>broken")); err == nil {
			t.Fatal("expected error for malformed xml, got nil")
		} else {
			t.Logf("malformed xml error: %v", err)
		}
	})
}
