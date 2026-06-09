package contentsafety

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHConfigPolicyDetectsExcludedMaterialWithoutBlockingNormalDirectives(t *testing.T) {
	t.Parallel()

	safe := []string{
		"Host github.com\n  HostName github.com\n  IdentityFile ~/.ssh/id_ed25519\n",
		"Host work\n  Include ~/.ssh/config.d/*.conf\n  CertificateFile ~/.ssh/id_ed25519-cert.pub\n",
		"Host proxy\n  ProxyCommand ssh -W %h:%p bastion\n  Match exec \"test -f ~/.ssh/id_ed25519\"\n",
	}
	for _, body := range safe {
		_, ok := Detect(Input{Policy: PolicySSHConfigObviousSecrets, Value: []byte(body), SettingRef: "ssh:config", SettingID: "config", ResourceID: "config"})
		require.False(t, ok, body)
	}

	tests := []struct {
		name     string
		body     string
		category string
		pattern  string
	}{
		{name: "private key", body: "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----\n", category: "private-key", pattern: "private_key_header"},
		{name: "public key", body: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM user@example\n", category: CategoryPublicKey, pattern: "openssh-public-key-line"},
		{name: "certificate", body: "ssh-ed25519-cert-v01@openssh.com AAAAC3NzaC1lZDI1NTE5AAAAIMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM user@example\n", category: CategoryCertificate, pattern: "openssh-certificate-line"},
		{name: "known hosts", body: "|1|abcdefghijklmnop=|qrstuvwxyzabcdef= ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM\n", category: CategoryKnownHosts, pattern: "known-hosts-hashed-host"},
		{name: "authorized keys with option", body: "from=\"10.0.0.0/8\" ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM user@example\n", category: CategoryAuthorizedKeys, pattern: "authorized-keys-optioned-public-key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			finding, ok := Detect(Input{Policy: PolicySSHConfigObviousSecrets, Value: []byte(tc.body), SettingRef: "ssh:config", SettingID: "config", ResourceID: "config"})
			require.True(t, ok)
			require.Equal(t, tc.category, finding.Category)
			require.Equal(t, tc.pattern, finding.PatternID)
			require.NotContains(t, finding.PatternID, strings.TrimSpace(tc.body))
		})
	}
}

func TestEmptyOrUnknownPolicyDoesNotDetect(t *testing.T) {
	t.Parallel()

	for _, policy := range []string{"", "unknown"} {
		_, ok := Detect(Input{Policy: policy, Value: []byte("-----BEGIN OPENSSH PRIVATE KEY-----")})
		require.False(t, ok)
	}
}
