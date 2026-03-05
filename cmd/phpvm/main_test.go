package main

import "testing"

func TestParseSemver(t *testing.T) {
	got := parseSemver("8.2.30")
	if got[0] != 8 || got[1] != 2 || got[2] != 30 {
		t.Fatalf("unexpected parse: %#v", got)
	}
}

func TestSemverLess(t *testing.T) {
	if !semverLess("8.2.1", "8.2.2") {
		t.Fatal("expected 8.2.1 < 8.2.2")
	}
	if semverLess("8.2.10", "8.2.2") {
		t.Fatal("expected 8.2.10 > 8.2.2")
	}
}

func TestIsStableVersion(t *testing.T) {
	cases := map[string]bool{
		"8.2.30":        true,
		"8.5.0RC1":      false,
		"8.4.0beta1":    false,
		"8.1":           false,
		"8":             false,
		"not-a-version": false,
	}
	for in, expect := range cases {
		if got := isStableVersion(in); got != expect {
			t.Fatalf("isStableVersion(%q)=%v want %v", in, got, expect)
		}
	}
}
