package discovery

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
)

// parseXMLPlist parses an XML property list (Info.plist style) into a flat
// key→string map of its top-level dict. Binary plists and non-XML input are
// rejected: only the `<?xml` form is supported (stdlib encoding/xml only).
func parseXMLPlist(r io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("<?xml")) {
		return nil, fmt.Errorf("not an XML plist")
	}
	out := map[string]string{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var lastKey string
	inDict := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse plist: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "dict":
			if !inDict {
				inDict = true
			}
		case "key":
			if !inDict {
				continue
			}
			var k string
			if err := dec.DecodeElement(&k, &se); err != nil {
				return nil, fmt.Errorf("parse plist key: %w", err)
			}
			lastKey = k
		case "string", "integer", "real":
			if !inDict || lastKey == "" {
				continue
			}
			var v string
			if err := dec.DecodeElement(&v, &se); err != nil {
				return nil, fmt.Errorf("parse plist value: %w", err)
			}
			out[lastKey] = v
			lastKey = ""
		}
	}
	return out, nil
}
