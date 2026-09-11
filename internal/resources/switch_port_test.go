package resources

import "testing"

func TestSwitchPortIDNormalizesMACWithoutChangingConfiguration(t *testing.T) {
	got := switchPortID("site1", "d8:44:89:38:c6:c0", 2)
	want := "site1:D8-44-89-38-C6-C0:2"
	if got != want {
		t.Errorf("switchPortID = %q, want %q", got, want)
	}
}
