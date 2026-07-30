package secretscan

import "testing"

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		kind string
	}{
		{name: "ordinary source", text: `logger.Info("server starting")`},
		{name: "secret assignment", text: `api_key := "company-secret-value"`, kind: "credential assignment"},
		{name: "private key", text: "-----BEGIN PRIVATE KEY-----", kind: "private key"},
		{name: "github token", text: "ghp_abcdefghijklmnopqrstuvwxyz", kind: "github token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			kind, found := Detect(test.text)
			if test.kind == "" {
				if found || kind != "" {
					t.Fatalf("Detect() = %q, %v", kind, found)
				}
				return
			}
			if !found || kind != test.kind {
				t.Fatalf("Detect() = %q, %v, want %q", kind, found, test.kind)
			}
		})
	}
}

func TestDetectCommonConfigFormats(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"API_KEY=actual-secret-value",
		`{"api_key":"actual-secret-value"}`,
		"password: actual-secret-value",
	} {
		if kind, found := Detect(input); !found || kind != "credential assignment" {
			t.Errorf("Detect(%q) = %q, %v", input, kind, found)
		}
	}
}

func TestDetectIgnoresDocumentedPlaceholders(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"API_KEY=your-api-key-here",
		`{"api_key":"placeholder-value"}`,
		"password: change-me-please",
		"password: authentication",
		"ClientSecret: 00000000000000000000000000000000",
	} {
		if kind, found := Detect(input); found {
			t.Errorf("Detect(%q) = %q, true; want placeholder ignored", input, kind)
		}
	}
}

func TestDetectDoesNotTreatMixedNumericCredentialAsPlaceholder(t *testing.T) {
	t.Parallel()

	const input = "ClientSecret: 00000000000000000000000000000001"
	if kind, found := Detect(input); !found || kind != "credential assignment" {
		t.Fatalf("Detect(%q) = %q, %v, want credential assignment", input, kind, found)
	}
}

func TestDetectIgnoresRuntimeSelectorAssignments(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"server.Password = options.password",
		"client.PrivateKey = key.String",
		"telegram.Token = flags.telegramToken",
		"apiKey := config.Sendgrid.ApiKey",
	} {
		if kind, found := Detect(input); found {
			t.Errorf("Detect(%q) = %q, true; want runtime selector ignored", input, kind)
		}
	}
}

func TestDetectKeepsQuotedDottedCredentialLiteralFailClosed(t *testing.T) {
	t.Parallel()

	const input = `password: "company.prod.secret"`
	if kind, found := Detect(input); !found || kind != "credential assignment" {
		t.Fatalf("Detect(%q) = %q, %v, want credential assignment", input, kind, found)
	}
}
