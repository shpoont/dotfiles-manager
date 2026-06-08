package secretpolicy

import "testing"

func TestDetectsKnownSecretPatterns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		value     string
		patternID string
	}{
		{name: "pem", value: "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----", patternID: "private_key_header"},
		{name: "escaped pem", value: "-----BEGIN RSA PRIVATE KEY-----\\nabc\\n-----END RSA PRIVATE KEY-----", patternID: "private_key_header"},
		{name: "github", value: "ghp_abcdefghijklmnopqrstuvwxyzABCDEFGH123456", patternID: "github_token"},
		{name: "github fine grained", value: "github_pat_" + repeat("A", 90), patternID: "github_fine_grained_pat"},
		{name: "gitlab", value: "glpat-abcdefghijklmnopqrstuvwxyz12", patternID: "gitlab_pat"},
		{name: "slack", value: "xoxb-not-a-real-token-value-abcdef", patternID: "slack_token"},
		{name: "openai", value: "sk-proj-abcdefghijklmnopqrstuvwxyz1234567890", patternID: "openai_api_key"},
		{name: "aws", value: "AKIAIOSFODNN7EXAMPLE", patternID: "aws_access_key_id"},
		{name: "google", value: "AIza" + repeat("A", 35), patternID: "google_api_key"},
		{name: "stripe", value: "sk_live_" + repeat("a", 24), patternID: "stripe_live_secret"},
		{name: "jwt", value: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnopqrstuvwxyz", patternID: "jwt_like"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			finding, ok := Detect(Input{Value: tc.value, SettingID: "user.email", SettingRef: "git:user.email"})
			if !ok {
				t.Fatalf("expected finding for %s", tc.name)
			}
			if finding.PatternID != tc.patternID {
				t.Fatalf("pattern = %s, want %s", finding.PatternID, tc.patternID)
			}
		})
	}
}

func TestDetectsHighEntropyOnlyInSensitiveContext(t *testing.T) {
	t.Parallel()

	value := "A9bC7dE8fG1hJ2kL3mN4pQ5r"
	finding, ok := Detect(Input{Value: value, SettingID: "api_token", SettingRef: "example:api_token"})
	if !ok {
		t.Fatal("expected high-entropy finding in sensitive context")
	}
	if finding.PatternID != "sensitive_context_high_entropy" {
		t.Fatalf("pattern = %s", finding.PatternID)
	}

	_, ok = Detect(Input{Value: value, SettingID: "theme", SettingRef: "example:theme"})
	if ok {
		t.Fatal("did not expect high-entropy finding without sensitive context")
	}
}

func TestBenignValuesAreNotDetected(t *testing.T) {
	t.Parallel()

	values := []Input{
		{Value: "leonid@example.com", SettingID: "user.email", SettingRef: "git:user.email"},
		{Value: "Leonid Komarovsky", SettingID: "user.name", SettingRef: "git:user.name"},
		{Value: "https://example.com/docs/theme", SettingID: "homepage", SettingRef: "app:homepage"},
		{Value: "nord-dark", SettingID: "theme", SettingRef: "app:theme"},
		{Value: "this-is-a-long-but-human-readable-token-label", SettingID: "token_label", SettingRef: "app:token_label"},
		{Value: "sk-test", SettingID: "publishable_key_label", SettingRef: "stripe:publishable_key_label"},
	}
	for _, input := range values {
		input := input
		t.Run(input.SettingRef, func(t *testing.T) {
			t.Parallel()
			if finding, ok := Detect(input); ok {
				t.Fatalf("unexpected finding: %+v", finding)
			}
		})
	}
}

func repeat(value string, count int) string {
	out := ""
	for i := 0; i < count; i++ {
		out += value
	}
	return out
}
