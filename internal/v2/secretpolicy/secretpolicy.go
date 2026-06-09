package secretpolicy

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

const (
	CategoryCredential = "credential"
	CategoryPrivateKey = "private-key"
	CategoryToken      = "token"
	CategoryAPIKey     = "api-key"
	CategoryJWT        = "jwt"
	CategoryEntropy    = "context-entropy"
)

type Input struct {
	Value      string
	SettingRef string
	SettingID  string
	ResourceID string
}

type Finding struct {
	PatternID string `json:"patternId"`
	Category  string `json:"category"`
}

type pattern struct {
	id       string
	category string
	re       *regexp.Regexp
}

var exactPatterns = []pattern{
	{id: "private_key_header", category: CategoryPrivateKey, re: regexp.MustCompile(`(?i)-----BEGIN[ A-Z0-9_-]*PRIVATE KEY-----`)},
	{id: "openssh_private_key", category: CategoryPrivateKey, re: regexp.MustCompile(`(?i)OPENSSH PRIVATE KEY`)},
	{id: "github_token", category: CategoryToken, re: regexp.MustCompile(`\bgh[oprsu]_[A-Za-z0-9_]{36,}\b`)},
	{id: "github_fine_grained_pat", category: CategoryToken, re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{40,}\b`)},
	{id: "gitlab_pat", category: CategoryToken, re: regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`)},
	{id: "slack_token", category: CategoryToken, re: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{20,}\b`)},
	{id: "openai_api_key", category: CategoryAPIKey, re: regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{24,}\b`)},
	{id: "aws_access_key_id", category: CategoryCredential, re: regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)},
	{id: "google_api_key", category: CategoryAPIKey, re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{id: "stripe_live_secret", category: CategoryAPIKey, re: regexp.MustCompile(`\b(?:sk_live|rk_live)_[0-9A-Za-z]{16,}\b`)},
	{id: "jwt_like", category: CategoryJWT, re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{16,}\b`)},
	{id: "bearer_token", category: CategoryToken, re: regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{24,}\b`)},
}

var sensitiveContextPattern = regexp.MustCompile(`(?i)(^|[._:/-])(api[-_]?key|auth|bearer|client[-_]?secret|cookie|credential|password|passwd|private[-_]?key|secret|session|token|access[-_]?key)($|[._:/-])`)

func Detect(input Input) (Finding, bool) {
	trimmed := strings.TrimSpace(input.Value)
	if trimmed == "" {
		return Finding{}, false
	}
	for _, variant := range scanVariants(trimmed) {
		for _, candidate := range exactPatterns {
			if candidate.re.MatchString(variant) {
				return Finding{PatternID: candidate.id, Category: candidate.category}, true
			}
		}
	}
	if !hasSensitiveContext(input) {
		return Finding{}, false
	}
	for _, token := range entropyTokens(trimmed) {
		if looksHighEntropySecret(token) {
			return Finding{PatternID: "sensitive_context_high_entropy", Category: CategoryEntropy}, true
		}
	}
	return Finding{}, false
}

func scanVariants(value string) []string {
	variants := []string{value}
	unescaped := strings.ReplaceAll(value, `\r\n`, "\n")
	unescaped = strings.ReplaceAll(unescaped, `\n`, "\n")
	unescaped = strings.ReplaceAll(unescaped, `\t`, "\t")
	if unescaped != value {
		variants = append(variants, unescaped)
	}
	compactWhitespace := regexp.MustCompile(`[ \t\r\n]+`).ReplaceAllString(unescaped, " ")
	if compactWhitespace != value && compactWhitespace != unescaped {
		variants = append(variants, compactWhitespace)
	}
	return variants
}

func hasSensitiveContext(input Input) bool {
	parts := []string{input.SettingID, input.SettingRef, input.ResourceID}
	for _, part := range parts {
		if sensitiveContextPattern.MatchString(part) {
			return true
		}
	}
	return false
}

func entropyTokens(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' && r != '/' && r != '+' && r != '='
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.Trim(field, " .,:;()[]{}<>\"'")
		if len(trimmed) >= 24 {
			out = append(out, trimmed)
		}
	}
	return out
}

func looksHighEntropySecret(value string) bool {
	if len(value) < 24 || len(value) > 4096 {
		return false
	}
	classes := 0
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	for _, present := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if present {
			classes++
		}
	}
	if classes < 3 {
		return false
	}
	return shannonEntropy(value) >= 3.5
}

func shannonEntropy(value string) float64 {
	if value == "" {
		return 0
	}
	counts := map[rune]int{}
	var total int
	for _, r := range value {
		counts[r]++
		total++
	}
	var entropy float64
	for _, count := range counts {
		p := float64(count) / float64(total)
		entropy -= p * math.Log2(p)
	}
	return entropy
}
