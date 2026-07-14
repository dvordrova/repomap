package report

import _ "embed"

const (
	elkJSVersion      = "0.11.1"
	elkJSBundleSHA256 = "20dd2114d683ce758b3ce19bcc56e28a504a617b0d280f760407c37314631d0e"
)

//go:embed assets/elkjs/elk.bundled.js
var elkJSBundledJS string

//go:embed assets/elkjs/LICENSE.md
var elkJSLicense string

//go:embed assets/elkjs/ATTRIBUTION.txt
var elkJSAttribution string
