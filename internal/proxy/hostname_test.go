package proxy_test

import (
	"testing"

	"github.com/astraqcd/athena-proxy/internal/proxy"
)

func TestNormalizeHostnameAcceptsAChallengeHostname(t *testing.T) {
	const hostname = "s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz"

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

func TestNormalizeHostnameRejects(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"bare label":       "localhost",
		"scheme":           "https://s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz",
		"port":             "s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz:443",
		"path":             "s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz/pwn",
		"uppercase":        "S8JS81P52QT5SIBPGDWRJHIX.challs.ctf-platform.xyz",
		"trailing dot":     "s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz.",
		"another domain":   "s8js81p52qt5sibpgdwrjhix.challs.example.com",
		"missing challs":   "s8js81p52qt5sibpgdwrjhix.ctf-platform.xyz",
		"id too short":     "s8js81p52qt5sibpgdwrjhi.challs.ctf-platform.xyz",
		"id too long":      "s8js81p52qt5sibpgdwrjhixx.challs.ctf-platform.xyz",
		"id starts digit":  "18js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz",
		"id has a hyphen":  "s8js81p52qt5sibpgdwrjh-x.challs.ctf-platform.xyz",
		"extra label":      "a.s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz",
		"base64url id":     "s8js81p52qt5sibpgdwrjh_x.challs.ctf-platform.xyz",
		"wildcard":         "*.challs.ctf-platform.xyz",
		"embedded newline": "s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz\nevil",
	}

	for name, input := range cases {
		if got, err := proxy.NormalizeHostname(input); err == nil {
			t.Errorf("%s: NormalizeHostname(%q) = %q, want an error", name, input, got)
		}
	}
}

func TestShortHostnameTruncatesOnlyTheID(t *testing.T) {
	cases := map[string]string{
		"s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz": "s8js81p5….challs.ctf-platform.xyz",
		"pwn.challs.ctf-platform.xyz":                      "pwn.challs.ctf-platform.xyz",
		"ctf-platform.xyz":                                 "ctf-platform.xyz",
	}

	for input, want := range cases {
		if got := proxy.ShortHostname(input); got != want {
			t.Errorf("ShortHostname(%q) = %q, want %q", input, got, want)
		}
	}
}
