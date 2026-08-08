package proxy

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	Domain    = "tcp.challs.ctf-platform.xyz"
	webDomain = "web.challs.ctf-platform.xyz"
)

const labelPattern = `[a-z][a-z0-9]{23}(-(?:[1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]))?`

var (
	hostnamePattern    = regexp.MustCompile(`^` + labelPattern + `\.` + regexp.QuoteMeta(Domain) + `$`)
	webHostnamePattern = regexp.MustCompile(`^` + labelPattern + `\.` + regexp.QuoteMeta(webDomain) + `$`)
)

func NormalizeHostname(raw string) (string, error) {
	hostname := strings.TrimSpace(raw)
	if hostnamePattern.MatchString(hostname) {
		return hostname, nil
	}
	if webHostnamePattern.MatchString(hostname) {
		return "", fmt.Errorf(
			"%q is a web challenge. Open https://%s in a browser instead of proxying it",
			hostname, hostname)
	}
	return "", fmt.Errorf("%q is not a challenge hostname, which looks like <id>.%s", hostname, Domain)
}

func ShortHostname(hostname string) string {
	label, rest, found := strings.Cut(hostname, ".")
	if !found || len(label) <= 12 {
		return hostname
	}
	return label[:8] + "…." + rest
}
