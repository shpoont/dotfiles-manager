package contentsafety

import (
	"regexp"
	"strings"

	"github.com/shpoont/dotfiles-manager/internal/v2/secretpolicy"
)

const (
	PolicySSHConfigObviousSecrets = "ssh-config-obvious-secrets"
)

const (
	CategorySecret         = "secret"
	CategoryPrivateKey     = "private-key"
	CategoryPublicKey      = "ssh-public-key"
	CategoryCertificate    = "ssh-certificate"
	CategoryKnownHosts     = "ssh-known-hosts"
	CategoryAuthorizedKeys = "ssh-authorized-keys"
)

type Input struct {
	Policy     string
	Value      []byte
	SettingRef string
	SettingID  string
	ResourceID string
}

type Finding struct {
	Category  string
	PatternID string
}

type pattern struct {
	id       string
	category string
	re       *regexp.Regexp
}

var sshConfigExcludedPatterns = []pattern{
	{
		id:       "known-hosts-hashed-host",
		category: CategoryKnownHosts,
		re:       regexp.MustCompile(`(?m)^\s*\|1\|[A-Za-z0-9+/=]+\|[A-Za-z0-9+/=]+\s+(?:ssh-ed25519|ssh-rsa|ecdsa-sha2-[A-Za-z0-9_-]+)\s+[A-Za-z0-9+/=]{16,}(?:\s|$)`),
	},
	{
		id:       "known-hosts-host-key",
		category: CategoryKnownHosts,
		re:       regexp.MustCompile(`(?m)^\s*(?:[A-Za-z0-9_.:,*?\[\]-]+)(?:,[A-Za-z0-9_.:,*?\[\]-]+)*\s+(?:ssh-ed25519|ssh-rsa|ecdsa-sha2-[A-Za-z0-9_-]+)\s+[A-Za-z0-9+/=]{16,}(?:\s|$)`),
	},
	{
		id:       "openssh-certificate-line",
		category: CategoryCertificate,
		re:       regexp.MustCompile(`(?m)^\s*(?:[A-Za-z0-9_-]+(?:=(?:"[^"\n]*"|[^\s"]+))?,?\s+)*(?:ssh-rsa|ssh-ed25519|ecdsa-sha2-[A-Za-z0-9_-]+|sk-ssh-[A-Za-z0-9@._-]+)-cert-v01@openssh\.com\s+[A-Za-z0-9+/=]{16,}(?:\s|$)`),
	},
	{
		id:       "authorized-keys-optioned-public-key",
		category: CategoryAuthorizedKeys,
		re:       regexp.MustCompile(`(?m)^\s*[A-Za-z0-9_-]+(?:=(?:"[^"\n]*"|[^\s",]+))?(?:,[A-Za-z0-9_-]+(?:=(?:"[^"\n]*"|[^\s",]+))?)*\s+(?:ssh-ed25519|ssh-rsa|ecdsa-sha2-[A-Za-z0-9_-]+|sk-ssh-[A-Za-z0-9@._-]+)\s+[A-Za-z0-9+/=]{16,}(?:\s|$)`),
	},
	{
		id:       "openssh-public-key-line",
		category: CategoryPublicKey,
		re:       regexp.MustCompile(`(?m)^\s*(?:ssh-ed25519|ssh-rsa|ecdsa-sha2-[A-Za-z0-9_-]+|sk-ssh-[A-Za-z0-9@._-]+)\s+[A-Za-z0-9+/=]{16,}(?:\s|$)`),
	},
}

func Detect(input Input) (Finding, bool) {
	switch strings.TrimSpace(input.Policy) {
	case "":
		return Finding{}, false
	case PolicySSHConfigObviousSecrets:
		return detectSSHConfig(input)
	default:
		return Finding{}, false
	}
}

func detectSSHConfig(input Input) (Finding, bool) {
	value := string(input.Value)
	if finding, ok := secretpolicy.Detect(secretpolicy.Input{
		Value:      value,
		SettingRef: input.SettingRef,
		SettingID:  input.SettingID,
		ResourceID: input.ResourceID,
	}); ok {
		category := finding.Category
		if category == "" {
			category = CategorySecret
		}
		return Finding{Category: category, PatternID: finding.PatternID}, true
	}
	for _, candidate := range sshConfigExcludedPatterns {
		if candidate.re.MatchString(value) {
			return Finding{Category: candidate.category, PatternID: candidate.id}, true
		}
	}
	return Finding{}, false
}
