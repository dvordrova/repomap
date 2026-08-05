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

func TestDetectIgnoresCredentialShapedWordsInsideMessageTemplates(t *testing.T) {
	t.Parallel()

	// Real source legitimately mentions credential-shaped words in log and
	// error messages; a format template is prose, never a credential
	// assignment (regression from the casdoor Scout request rejection).
	for _, input := range []string{
		`fmt.Sprintf(i18n.Translate(lang, "token:Grant_type: %s is not supported in this application"), responseType)`,
		`fmt.Errorf("password: %s is invalid", name)`,
		`log.Printf("client_secret: %v", clientSecret)`,
		`PrivateKey:      tokenJwtPrivateKey,`,
		`Password:        myRuntimePassword,`,
		`clientSecret:    cfg.ClientSecret,`,
	} {
		if kind, found := Detect(input); found {
			t.Errorf("Detect(%q) = %q, true; want message template ignored", input, kind)
		}
	}
	// A real quoted credential literal must still fail closed.
	if kind, found := Detect(`token: "actual-secret-value-1234567890"`); !found || kind != "credential assignment" {
		t.Fatalf("Detect(quoted token) = %q, %v, want credential assignment", kind, found)
	}
}

func TestDetectSourceMaterialIgnoresCredentialAssignmentsButBlocksRealSecrets(t *testing.T) {
	t.Parallel()

	// Locally expanded production source legitimately contains credential-shaped
	// assignments (owner doctrine: a repo that mentions credentials is not our
	// reason to make Study unavailable). DetectSourceMaterial must pass them.
	for _, input := range []string{
		`Password:        "some-runtime-password-value",`,
		`ClientSecret:    "a-client-secret-value",`,
		`token = "an-opaque-token-value"`,
		`api_key := "a-test-api-key-value"`,
		`accessToken: "a-long-access-token-value"`,
	} {
		if kind, found := DetectSourceMaterial(input); found {
			t.Errorf("DetectSourceMaterial(%q) = %q, true; want credential assignment ignored", input, kind)
		}
	}
	// Real credential material always fails closed, even in source material.
	for _, input := range []string{
		"-----BEGIN RSA PRIVATE KEY-----\nMIIE",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789",
		"sk-ant-api03-abcdefghijklmnopqrstuvwxyz123456",
		"ghp_abcdefghijklmnopqrstuvwxyz1234567890",
		"AKIAIOSFODNN7EXAMPLE",
	} {
		if kind, found := DetectSourceMaterial(input); !found {
			t.Errorf("DetectSourceMaterial(%q) = %q, false; want real secret blocked", input, kind)
		}
	}
}

func TestDetectAlwaysIgnoresUnsafeOverride(t *testing.T) {
	restore := SetDisabled(true)
	defer restore()

	const input = `API_KEY="actual-secret-value"`
	if kind, found := Detect(input); found || kind != "" {
		t.Fatalf("Detect() = %q, %v while disabled", kind, found)
	}
	if kind, found := DetectAlways(input); !found || kind != "credential assignment" {
		t.Fatalf("DetectAlways() = %q, %v, want mandatory detection", kind, found)
	}
}

func TestClosedKindUsesOnlyBoundedCodes(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"private key":           ClosedKindPrivateKey,
		"bearer credential":     ClosedKindBearerCredential,
		"secret key":            ClosedKindSecretKey,
		"github token":          ClosedKindGitHubToken,
		"aws access key":        ClosedKindAWSAccessKey,
		"credential assignment": ClosedKindCredentialAssignment,
		"detector prose drift":  ClosedKindUnknown,
	}
	for input, want := range tests {
		if got := ClosedKind(input); got != want {
			t.Errorf("ClosedKind(%q) = %q, want %q", input, got, want)
		}
	}
}
