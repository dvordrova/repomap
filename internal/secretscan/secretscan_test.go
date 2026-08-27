package secretscan

import "testing"

func enableScan(t *testing.T) {
	t.Helper()
	restore := SetEnabled(true)
	t.Cleanup(restore)
}

func TestDetect(t *testing.T) {
	enableScan(t)

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
	enableScan(t)

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
	enableScan(t)

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
	enableScan(t)

	const input = "ClientSecret: 00000000000000000000000000000001"
	if kind, found := Detect(input); !found || kind != "credential assignment" {
		t.Fatalf("Detect(%q) = %q, %v, want credential assignment", input, kind, found)
	}
}

func TestDetectIgnoresRuntimeSelectorAssignments(t *testing.T) {
	enableScan(t)

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
	enableScan(t)

	const input = `password: "company.prod.secret"`
	if kind, found := Detect(input); !found || kind != "credential assignment" {
		t.Fatalf("Detect(%q) = %q, %v, want credential assignment", input, kind, found)
	}
}

func TestDetectIgnoresCredentialShapedWordsInsideMessageTemplates(t *testing.T) {
	enableScan(t)

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

func TestDetectDistinguishesBearerProseFromOpaqueCredentials(t *testing.T) {
	enableScan(t)

	for _, input := range []string{
		`{"title":"Bearer authentication"}`,
		`{"question":"How does Bearer authorization reach the handler?"}`,
		`Bearer configuration`,
		`Bearer integration`,
		`Bearer implementation`,
		`Bearer verification`,
		`Bearer introspection`,
		`Bearer compatibility`,
		`Bearer Authentication`,
		`Bearer authentication-based`,
	} {
		if kind, found := Detect(input); found {
			t.Errorf("Detect(%q) = %q, true; want ordinary Bearer prose", input, kind)
		}
	}
	for _, input := range []string{
		`Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789`,
		`{"response":"Bearer company-secret-token-value"}`,
		`const auth = "Bearer abcdefghijklmnop"`,
	} {
		if kind, found := Detect(input); !found || kind != "bearer credential" {
			t.Errorf("Detect(%q) = %q, %v; want bearer credential", input, kind, found)
		}
	}
}

func TestCredentialScanningIsDisabledByDefault(t *testing.T) {
	const input = `API_KEY="actual-secret-value"`
	if kind, found := Detect(input); found || kind != "" {
		t.Fatalf("detection = %q, %v while opt-in is disabled", kind, found)
	}
}

func TestDetectPersistenceSensitiveIsNarrowAndAlwaysOn(t *testing.T) {
	restore := SetEnabled(false)
	t.Cleanup(restore)

	detected := []string{
		"Authorization: Bearer abcdefghijklmnop",
		`const headers = { Authorization: bearerToken }`,
		"const header = `Authorization: Basic opaque-provider-value`",
		`{"api_key":"opaque-provider-value-1234"}`,
		`{"content":"api_key = \"opaque-provider-value-1234\""}`,
		`{"error":"sk-secret-shaped-provider-output"}`,
		"-----BEGIN PRIVATE KEY-----",
	}
	for _, input := range detected {
		if kind, found := DetectPersistenceSensitive(input); !found || kind == "" {
			t.Errorf("DetectPersistenceSensitive(%q) = %q, %v; want closed kind", input, kind, found)
		}
		if kind, found := Detect(input); found || kind != "" {
			t.Errorf("opt-in Detect(%q) = %q, %v while disabled", input, kind, found)
		}
	}

	for _, input := range []string{
		`logger.Info("Bearer authentication configured")`,
		`apiKey := config.Sendgrid.ApiKey`,
		`api_key = os.Getenv`,
		`{"title":"Authorization middleware and api_key settings"}`,
		`{"ref":"export:src/server/middleware/authorization:authorize"}`,
		`Responsibility representative: Authentication and authorization: go.etcd.io/etcd/server/v3/auth: Authenticate`,
	} {
		if kind, found := DetectPersistenceSensitive(input); found {
			t.Errorf("DetectPersistenceSensitive(%q) = %q, true; want ordinary code/prose", input, kind)
		}
	}
}
