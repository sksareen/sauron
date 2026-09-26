package daemon

import "testing"

func TestParseLSAppInfo(t *testing.T) {
	out := "\"CFBundleIdentifier\"=\"com.todesktop.230313mzl4w4u92\" \"LSDisplayName\"=\"Cursor\"\n"
	name, bundle := parseLSAppInfo(out)
	if name != "Cursor" || bundle != "com.todesktop.230313mzl4w4u92" {
		t.Fatalf("got name=%q bundle=%q", name, bundle)
	}
}

func TestParseLSAppInfoFallsBackToBundleName(t *testing.T) {
	out := "\"CFBundleName\"=\"Terminal\"\n\"CFBundleIdentifier\"=\"com.apple.Terminal\"\n"
	name, bundle := parseLSAppInfo(out)
	if name != "Terminal" || bundle != "com.apple.Terminal" {
		t.Fatalf("got name=%q bundle=%q", name, bundle)
	}
}

func TestParseFrontAppName(t *testing.T) {
	got := parseFrontAppName(`ASN:0x0-0x4a04a: "Cursor"`)
	if got != "Cursor" {
		t.Fatalf("got %q", got)
	}
	if parseFrontAppName("ASN:0x0-0x4a04a:") != "" {
		t.Fatal("expected empty name")
	}
}
