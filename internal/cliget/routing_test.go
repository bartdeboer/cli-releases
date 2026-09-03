package cliget

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestGoClirHelpIsProviderFreeAndContextual(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"install", "--help"}, {"bootstrap", "help"}} {
		var out, errOut strings.Builder
		if err := Run(context.Background(), args, &out, &errOut); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if out.Len() == 0 || errOut.Len() != 0 {
			t.Fatalf("%v out=%q err=%q", args, out.String(), errOut.String())
		}
	}
}
func TestRoutesRejectMissingAndFlagLookingTool(t *testing.T) {
	for _, args := range [][]string{{"install"}, {"install", "--json"}, {"unknown"}, {"bootstrap", "extra"}} {
		if err := Run(context.Background(), args, io.Discard, io.Discard); err == nil {
			t.Fatalf("accepted %#v", args)
		}
	}
}
func TestStrictOptionsRejectDuplicatesUniformly(t *testing.T) {
	base := []string{"--version", "v1.0.0"}
	cases := [][]string{append(append([]string{}, base...), "--version", "v1.0.1"), append(append([]string{}, base...), "--json", "--json"), append(append([]string{}, base...), "--overwrite", "--overwrite"), append(append([]string{}, base...), "--bin-dir", "/a", "--bin-dir", "/b")}
	for _, args := range cases {
		if _, err := parse("tool", args); err == nil {
			t.Fatalf("accepted %#v", args)
		}
	}
}
func TestMissingOptionValueAndJSONStreamParity(t *testing.T) {
	if _, err := parse("tool", []string{"--version"}); err == nil {
		t.Fatal("accepted missing value")
	}
	if !JSONRequested([]string{"install", "tool", "--json"}) {
		t.Fatal("lost json detection")
	}
	var b strings.Builder
	if err := WriteError(&b, context.Canceled); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"apiVersion":"cli-get.output/v1"`) || !strings.Contains(b.String(), `"status":"error"`) {
		t.Fatal(b.String())
	}
}
