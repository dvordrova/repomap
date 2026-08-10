package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestD268SelectedComponentHoverStateMatrix(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("templates", "architecture_canvas.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)

	matrix := []struct {
		state    string
		selector string
		style    string
	}{
		{
			state:    "pointer over an ordinary non-dimmed component",
			selector: ".rm-arch__component:not(.is-dimmed):not(.is-selected) .rm-arch__component-card:hover",
			style:    "box-shadow: 0 7px 19px",
		},
		{
			state:    "pointer over the selected component retains selected feedback",
			selector: ".rm-arch__component.is-selected .rm-arch__component-card:hover",
			style:    "border-color: var(--rm-arch-blue)",
		},
		{
			state:    "keyboard focus remains independently visible",
			selector: ".rm-arch button:focus-visible",
			style:    "outline: 3px solid",
		},
	}
	for _, test := range matrix {
		t.Run(test.state, func(t *testing.T) {
			body := cssRuleBodyD268(t, css, test.selector)
			if !strings.Contains(body, test.style) {
				t.Fatalf("%s rule %q does not contain %q: %s", test.state, test.selector, test.style, body)
			}
		})
	}

	for _, forbidden := range []string{
		"\n.rm-arch__component-card:hover {",
		".rm-arch__component.is-dimmed .rm-arch__component-card:hover",
	} {
		if strings.Contains(css, forbidden) {
			t.Fatalf("dimmed components still inherit hover elevation through %q", forbidden)
		}
	}
}

func TestD268MobileStudyContentsKeepsOneWidthBoundedDOMList(t *testing.T) {
	styleRaw, err := os.ReadFile(filepath.Join("templates", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	style := string(styleRaw)
	mediaStart := strings.Index(style, "@media (max-width: 640px) {")
	mediaEnd := strings.Index(style[mediaStart:], "/* Decision 229 D6 Study progressive disclosure")
	if mediaStart < 0 || mediaEnd < 0 {
		t.Fatal("640px report media block is missing")
	}
	mobile := style[mediaStart : mediaStart+mediaEnd]
	for selector, required := range map[string][]string{
		".rm-study-theme-contents":         {"box-sizing: border-box", "max-width: 100%", "min-width: 0"},
		".rm-study-theme-contents__list":   {"columns: 1", "max-width: 100%"},
		".rm-study-theme-contents__action": {"max-width: 100%", "overflow-wrap: anywhere", "width: 100%"},
	} {
		body := cssRuleBodyD268(t, mobile, selector)
		for _, token := range required {
			if !strings.Contains(body, token) {
				t.Errorf("mobile %s rule is missing %q: %s", selector, token, body)
			}
		}
	}

	scriptRaw, err := os.ReadFile(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptRaw)
	if strings.Count(script, "var contentList = el('ol', 'rm-study-theme-contents__list')") != 1 ||
		strings.Count(script, "contentList.appendChild(item)") != 1 {
		t.Fatal("responsive Study contents must reuse one ordered DOM list and its existing tab order")
	}
}

func cssRuleBodyD268(t *testing.T, css string, selector string) string {
	t.Helper()
	start := strings.Index(css, selector)
	if start < 0 {
		t.Fatalf("CSS selector %q is missing", selector)
	}
	open := strings.Index(css[start:], "{")
	if open < 0 {
		t.Fatalf("CSS selector %q has no rule body", selector)
	}
	open += start
	close := strings.Index(css[open:], "}")
	if close < 0 {
		t.Fatalf("CSS selector %q has an unterminated rule body", selector)
	}
	return css[open+1 : open+close]
}
