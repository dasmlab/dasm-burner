package ui

import (
	"strings"
	"testing"
)

func TestParseLoginPasteOC(t *testing.T) {
	raw := `oc login --token=sha256~Ij630SUiZ-Y4ABCDEFGHIJKLMNOP --server=https://api.2026-prod-1.ocp.dasmlab.org:6443`
	p, err := ParseLoginPaste(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != "oc" {
		t.Fatalf("format %s", p.Format)
	}
	if p.Server != "https://api.2026-prod-1.ocp.dasmlab.org:6443" {
		t.Fatalf("server %s", p.Server)
	}
	if p.Token != "sha256~Ij630SUiZ-Y4ABCDEFGHIJKLMNOP" {
		t.Fatalf("token %s", p.Token)
	}
	if p.Name != "2026-prod-1" {
		t.Fatalf("name %s", p.Name)
	}
}

func TestParseLoginPasteCurl(t *testing.T) {
	raw := `curl -H "Authorization: Bearer sha256~Ij630SUiZ-Y4ABCDEFGHIJKLMNOP" "https://api.2026-prod-1.ocp.dasmlab.org:6443/apis/user.openshift.io/v1/users/~"`
	p, err := ParseLoginPaste(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != "curl" {
		t.Fatalf("format %s", p.Format)
	}
	if p.Server != "https://api.2026-prod-1.ocp.dasmlab.org:6443" {
		t.Fatalf("server %s", p.Server)
	}
}

func TestParseLoginPasteBoth(t *testing.T) {
	raw := `
Your API token is sha256~ignored-here
Log in with this token
oc login --token=sha256~Ij630SUiZ-Y4ABCDEFGHIJKLMNOP --server=https://api.2026-prod-1.ocp.dasmlab.org:6443
Use this token directly against the API
curl -H "Authorization: Bearer sha256~Ij630SUiZ-Y4ABCDEFGHIJKLMNOP" "https://api.2026-prod-1.ocp.dasmlab.org:6443/apis/user.openshift.io/v1/users/~"
`
	p, err := ParseLoginPaste(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != "mixed" {
		t.Fatalf("format %s", p.Format)
	}
	if p.Server != "https://api.2026-prod-1.ocp.dasmlab.org:6443" {
		t.Fatalf("server %s", p.Server)
	}
	if !strings.Contains(p.Token, "Ij630") {
		t.Fatalf("token missing")
	}
}

func TestParseLoginPasteCurlOnlyPieces(t *testing.T) {
	raw := `curl -k -H 'Authorization: Bearer sha256~ABCDEFGHIJKLMNOPQRSTUV' https://api.lab.example.com:6443/api`
	p, err := ParseLoginPaste(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Server != "https://api.lab.example.com:6443" {
		t.Fatalf("server %s", p.Server)
	}
}
