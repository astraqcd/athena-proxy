package proxy

import (
	"fmt"
	"regexp"
	"strings"
)

const Domain = "challs.ctf-platform.xyz"

var hostnamePattern = regexp.MustCompile(`^[a-z][a-z0-9]{23}\.` + regexp.QuoteMeta(Domain) + `$`)

func NormalizeHostname(raw string) (string, error) {
	hostname := strings.TrimSpace(raw)
	if !hostnamePattern.MatchString(hostname) {
		return "", fmt.Errorf("%q is not a challenge hostname, which looks like <id>.%s", hostname, Domain)
	}
	return hostname, nil
}

func ShortHostname(hostname string) string {
	label, rest, found := strings.Cut(hostname, ".")
	if !found || len(label) <= 12 {
		return hostname
	}
	return label[:8] + "…." + rest
}
