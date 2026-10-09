package report

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"strings"
)

// embeddedJSON changes only the physical representation of complete generated
// JSON. The ordinary browser bundle receives the original bytes before it runs.
// Smaller raw values remain raw; this choice never limits or drops evidence.
func embeddedJSON(id string, raw template.JS) (template.HTML, error) {
	switch id {
	case "rm-source-ids", "rm-ui-vocabulary", "rm-term-mentions", "rm-scene", "rm-page-data":
	default:
		return "", fmt.Errorf("report: unknown embedded JSON block %q", id)
	}
	var packed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&packed, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	// Hide strings.Reader.WriteTo: its io.WriteString fallback would convert
	// the whole remaining string to one []byte for gzip.Writer.
	reader := struct{ io.Reader }{strings.NewReader(string(raw))}
	if _, err := io.Copy(writer, reader); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	const attribute = ` data-rm-encoding="gzip-base64"`
	body, encoding := string(raw), ""
	if base64.StdEncoding.EncodedLen(packed.Len())+len(attribute) < len(body) {
		body, encoding = base64.StdEncoding.EncodeToString(packed.Bytes()), attribute
	}
	return template.HTML(`<script type="application/json" id="` + id + `"` + encoding + `>` + body + `</script>`), nil
}

func embeddedReportBoot() (template.JS, error) {
	data, err := reportTemplateFS.ReadFile("templates/boot.js")
	return template.JS(data), err
}

// The bundle consists exclusively of our embedded generated application assets.
// An unknown inert MIME type otherwise makes html/template HTML-escape its JS;
// emitting the complete element keeps its classic source bytes unchanged.
func embeddedReportBundle(raw template.JS) template.HTML {
	return template.HTML(`<script type="application/x-repomap-js" id="rm-report-app-js">` + string(raw) + `</script>`)
}
