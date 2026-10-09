package report

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"regexp"
	"strings"
	"testing"
)

func readEmbeddedJSON(t *testing.T, page []byte, id string) []byte {
	t.Helper()
	pattern := `<script type="application/json" id="` + regexp.QuoteMeta(id) + `"( data-rm-encoding="gzip-base64")?>([^<]*)</script>`
	parts := regexp.MustCompile(pattern).FindSubmatch(page)
	if len(parts) != 3 {
		t.Fatalf("missing complete embedded JSON %s", id)
	}
	if len(parts[1]) == 0 {
		return parts[2]
	}
	packed, err := base64.StdEncoding.DecodeString(string(parts[2]))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestEmbeddedJSONPreservesAllGeneratedBytes(t *testing.T) {
	for _, value := range []string{`null`, `[]`, `{"name":"東京 & Москва","source":"a.cljc:17:4","refs":["t1","t16"]}`, `{"rows":[` + strings.TrimSuffix(strings.Repeat(`{"name":"東京 & Москва","source":"a.cljc:17:4","refs":["t1","t16"]},`, 5000), ",") + `]}`} {
		for _, id := range []string{"rm-source-ids", "rm-ui-vocabulary", "rm-term-mentions", "rm-scene", "rm-page-data"} {
			raw := template.JS(value)
			first, err := embeddedJSON(id, raw)
			if err != nil {
				t.Fatal(err)
			}
			second, err := embeddedJSON(id, raw)
			if err != nil || first != second {
				t.Fatal("saved rendering has nondeterministic embedded bytes")
			}
			if !bytes.Equal(readEmbeddedJSON(t, []byte(first), id), []byte(raw)) {
				t.Fatal("embedded transport changed complete bytes")
			}
		}
	}
}

func TestInertBundlePreservesClassicSourceBytesThroughTemplate(t *testing.T) {
	const source = `var compare = a < b && c > d; var quote = "東京 & Москва";`
	tmpl, err := template.New("bundle").Funcs(template.FuncMap{"reportBundle": embeddedReportBundle}).Parse(`{{reportBundle .}}`)
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, template.JS(source)); err != nil {
		t.Fatal(err)
	}
	_, content, found := strings.Cut(rendered.String(), ">")
	if !found || strings.TrimSuffix(content, "</script>") != source {
		t.Fatalf("classic bundle source bytes changed: %s", rendered.String())
	}
	for _, bad := range []string{"&lt;", "&gt;", "&amp;", "&#34;"} {
		if strings.Contains(rendered.String(), bad) {
			t.Fatal(fmt.Sprintf("bundle acquired HTML entity %s", bad))
		}
	}
}
