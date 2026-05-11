package main

import "testing"

func TestSplitGlobalConfigArgRejectsBlankPath(t *testing.T) {
	cases := [][]string{
		{"--config", ""},
		{"--config="},
		{"-c", "  "},
	}
	for _, args := range cases {
		if _, _, err := splitGlobalConfigArg(args); err == nil {
			t.Fatalf("splitGlobalConfigArg(%v) error = nil, want error", args)
		}
	}
}

func TestSplitGlobalConfigArgAcceptsExplicitPath(t *testing.T) {
	path, args, err := splitGlobalConfigArg([]string{"--config", "custom.json", "cli", "config", "path"})
	if err != nil {
		t.Fatalf("splitGlobalConfigArg failed: %v", err)
	}
	if path != "custom.json" {
		t.Fatalf("path = %q, want custom.json", path)
	}
	if len(args) != 3 || args[0] != "cli" || args[1] != "config" || args[2] != "path" {
		t.Fatalf("args = %#v, want [cli config path]", args)
	}
}
