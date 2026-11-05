package resources

import (
	"strings"
	"testing"

	"github.com/filipegalo/terraform-provider-omada/internal/client"
)

func TestSwitchIDNormalizesMACWithoutChangingConfiguration(t *testing.T) {
	got := switchID("site1", "aa:bb:cc:dd:ee:ff")
	want := "site1:AA-BB-CC-DD-EE-FF"
	if got != want {
		t.Errorf("switchID = %q, want %q", got, want)
	}
}

// TestSplitSwitchImportID covers the ambiguity the import ID has to resolve:
// the site separator is a colon, and so is the separator of one MAC notation.
func TestSplitSwitchImportID(t *testing.T) {
	cases := map[string]struct {
		id       string
		wantSite string
		wantMAC  string
		wantOK   bool
	}{
		"bare dash-separated MAC":       {id: "AA-BB-CC-DD-EE-FF", wantMAC: "AA-BB-CC-DD-EE-FF", wantOK: true},
		"bare colon-separated MAC":      {id: "aa:bb:cc:dd:ee:ff", wantMAC: "aa:bb:cc:dd:ee:ff", wantOK: true},
		"site name and dash MAC":        {id: "Default:AA-BB-CC-DD-EE-FF", wantSite: "Default", wantMAC: "AA-BB-CC-DD-EE-FF", wantOK: true},
		"site name and colon MAC":       {id: "Default:aa:bb:cc:dd:ee:ff", wantSite: "Default", wantMAC: "aa:bb:cc:dd:ee:ff", wantOK: true},
		"site ID and dash MAC":          {id: "0123456789abcdef01234567:AA-BB-CC-DD-EE-FF", wantSite: "0123456789abcdef01234567", wantMAC: "AA-BB-CC-DD-EE-FF", wantOK: true},
		"empty":                         {id: ""},
		"site without MAC":              {id: "Default"},
		"empty site":                    {id: ":AA-BB-CC-DD-EE-FF"},
		"truncated MAC":                 {id: "Default:AA-BB-CC-DD-EE"},
		"mixed separators":              {id: "AA-BB:CC-DD:EE-FF"},
		"switch port import ID":         {id: "Default:AA-BB-CC-DD-EE-FF:2"},
		"MAC followed by trailing text": {id: "AA-BB-CC-DD-EE-FF-00"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			site, mac, ok := splitSwitchImportID(tc.id)
			if site != tc.wantSite || mac != tc.wantMAC || ok != tc.wantOK {
				t.Errorf("splitSwitchImportID(%q) = %q, %q, %t; want %q, %q, %t", tc.id, site, mac, ok, tc.wantSite, tc.wantMAC, tc.wantOK)
			}
		})
	}
}

// TestCheckSwitchNameAppliesTheAgileSeriesLimit guards the limit the schema
// cannot express: the same name attribute is 128 characters on a regular
// switch and 32 on an Agile Series one, counted in characters, not bytes.
func TestCheckSwitchNameAppliesTheAgileSeriesLimit(t *testing.T) {
	atLimit := strings.Repeat("ä", client.EasyManagedSwitchNameMaxLength)
	if err := checkSwitchName(atLimit, true); err != nil {
		t.Errorf("checkSwitchName rejected a %d-character name for an Agile Series switch: %v", client.EasyManagedSwitchNameMaxLength, err)
	}
	if err := checkSwitchName(atLimit+"x", true); err == nil {
		t.Errorf("checkSwitchName accepted a %d-character name for an Agile Series switch", client.EasyManagedSwitchNameMaxLength+1)
	}
	if err := checkSwitchName(atLimit+"x", false); err != nil {
		t.Errorf("checkSwitchName applied the Agile Series limit to a regular switch: %v", err)
	}
}
