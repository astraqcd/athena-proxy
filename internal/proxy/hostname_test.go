package proxy_test

import (
	"strings"
	"testing"

	"github.com/astraqcd/athena-proxy/internal/proxy"
)

func TestNormalizeHostnameAcceptsAChallengeHostname(t *testing.T) {
	const hostname = "s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz"

	for _, input := range []string{hostname, "  " + hostname + "  ", "\t" + hostname + "\n"} {
		got, err := proxy.NormalizeHostname(input)
		if err != nil {
			t.Errorf("NormalizeHostname(%q): %v", input, err)
			continue
		}
		if got != hostname {
			t.Errorf("NormalizeHostname(%q) = %q, want %q", input, got, hostname)
		}
	}
}

func TestNormalizeHostnameAcceptsAPortSuffix(t *testing.T) {
	for _, hostname := range []string{
		"s8js81p52qt5sibpgdwrjhix-1.tcp.challs.ctf-platform.xyz",
		"s8js81p52qt5sibpgdwrjhix-1337.tcp.challs.ctf-platform.xyz",
		"s8js81p52qt5sibpgdwrjhix-65535.tcp.challs.ctf-platform.xyz",
	} {
		if got, err := proxy.NormalizeHostname(hostname); err != nil || got != hostname {
			t.Errorf("NormalizeHostname(%q) = %q, %v", hostname, got, err)
		}
	}
}

func TestNormalizeHostnameSendsWebChallengesToABrowser(t *testing.T) {
	for _, hostname := range []string{
		"s8js81p52qt5sibpgdwrjhix.web.challs.ctf-platform.xyz",
		"s8js81p52qt5sibpgdwrjhix-8080.web.challs.ctf-platform.xyz",
	} {
		_, err := proxy.NormalizeHostname(hostname)
		if err == nil {
			t.Fatalf("NormalizeHostname(%q) must not register a web challenge", hostname)
		}
		if !strings.Contains(err.Error(), "browser") {
			t.Errorf("NormalizeHostname(%q) should point at a browser, got: %v", hostname, err)
		}
		if !strings.Contains(err.Error(), "https://"+hostname) {
			t.Errorf("NormalizeHostname(%q) should print the URL to open, got: %v", hostname, err)
		}
	}
}

func TestNormalizeHostnameRejects(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"bare label":        "localhost",
		"scheme":            "https://s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz",
		"port":              "s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz:443",
		"path":              "s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz/pwn",
		"uppercase":         "S8JS81P52QT5SIBPGDWRJHIX.tcp.challs.ctf-platform.xyz",
		"trailing dot":      "s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz.",
		"another domain":    "s8js81p52qt5sibpgdwrjhix.tcp.challs.example.com",
		"web subdomain":     "s8js81p52qt5sibpgdwrjhix.web.challs.ctf-platform.xyz",
		"missing challs":    "s8js81p52qt5sibpgdwrjhix.ctf-platform.xyz",
		"missing kind":      "s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz",
		"id too short":      "s8js81p52qt5sibpgdwrjhi.tcp.challs.ctf-platform.xyz",
		"id too long":       "s8js81p52qt5sibpgdwrjhixx.tcp.challs.ctf-platform.xyz",
		"id starts digit":   "18js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz",
		"id has a hyphen":   "s8js81p52qt5sibpgdwrjh-x.tcp.challs.ctf-platform.xyz",
		"extra label":       "a.s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz",
		"base64url id":      "s8js81p52qt5sibpgdwrjh_x.tcp.challs.ctf-platform.xyz",
		"wildcard":          "*.tcp.challs.ctf-platform.xyz",
		"port zero":         "s8js81p52qt5sibpgdwrjhix-0.tcp.challs.ctf-platform.xyz",
		"port too high":     "s8js81p52qt5sibpgdwrjhix-65536.tcp.challs.ctf-platform.xyz",
		"port leading zero": "s8js81p52qt5sibpgdwrjhix-080.tcp.challs.ctf-platform.xyz",
		"empty suffix":      "s8js81p52qt5sibpgdwrjhix-.tcp.challs.ctf-platform.xyz",
		"embedded newline":  "s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz\nevil",
	}

	for name, input := range cases {
		if got, err := proxy.NormalizeHostname(input); err == nil {
			t.Errorf("%s: NormalizeHostname(%q) = %q, want an error", name, input, got)
		}
	}
}

func TestShortHostnameTruncatesOnlyTheID(t *testing.T) {
	cases := map[string]string{
		"s8js81p52qt5sibpgdwrjhix.tcp.challs.ctf-platform.xyz": "s8js81p5….tcp.challs.ctf-platform.xyz",
		"pwn.tcp.challs.ctf-platform.xyz":                      "pwn.tcp.challs.ctf-platform.xyz",
		"ctf-platform.xyz":                                     "ctf-platform.xyz",
	}

	for input, want := range cases {
		if got := proxy.ShortHostname(input); got != want {
			t.Errorf("ShortHostname(%q) = %q, want %q", input, got, want)
		}
	}
}
