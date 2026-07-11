package report

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestEmbeddedELKJSAsset(t *testing.T) {
	t.Parallel()

	if elkJSBundledJS == "" {
		t.Fatal("embedded ELK.js browser bundle is empty")
	}
	gotSHA256 := fmt.Sprintf("%x", sha256.Sum256([]byte(elkJSBundledJS)))
	if gotSHA256 != elkJSBundleSHA256 {
		t.Fatalf("embedded ELK.js sha256 = %s, want pinned %s", gotSHA256, elkJSBundleSHA256)
	}
	if !strings.Contains(elkJSBundledJS, "SPDX-License-Identifier: EPL-2.0") {
		t.Fatal("embedded ELK.js bundle does not retain its EPL-2.0 license header")
	}
	if !strings.Contains(elkJSLicense, "# Eclipse Public License - v 2.0") {
		t.Fatal("embedded ELK.js license text is missing")
	}
	if !strings.Contains(elkJSAttribution, "Version: "+elkJSVersion) ||
		!strings.Contains(elkJSAttribution, "Source code for this version:") {
		t.Fatal("embedded ELK.js attribution does not identify the pinned version and source")
	}
}
