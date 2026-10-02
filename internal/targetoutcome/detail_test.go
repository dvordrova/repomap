package targetoutcome

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// A failure is shown in its own words, with the repository's paths relative
// and no host path: the home directory reads "~", any other absolute path
// keeps only its last element, and a URL or a relative path stays as
// written.
func TestFailureDetailReadsRelativeWithNoHostPath(t *testing.T) {
	const home = "/Users/someone"
	roots := []string{"/Users/someone/git/litestream", "/private/var/link/litestream", "/var/link/litestream"}
	for _, test := range []struct{ text, want string }{
		{
			"no build line compiles src/litestream-vfs.c; parsed with clang's defaults (exit status 1): src/litestream-vfs.c:1:10: fatal error: 'litestream-vfs.h' file not found",
			"no build line compiles src/litestream-vfs.c; parsed with clang's defaults (exit status 1): src/litestream-vfs.c:1:10: fatal error: 'litestream-vfs.h' file not found",
		},
		{"open /Users/someone/git/litestream/cmd/main.go: permission denied", "open cmd/main.go: permission denied"},
		{"go: cannot find main module in /private/var/link/litestream; see https://go.dev/ref/mod", "go: cannot find main module in .; see https://go.dev/ref/mod"},
		{
			"/Users/someone/go/pkg/mod/github.com/x/y@v1.2.3/z.go:4:2: undefined: q\n/usr/local/go/src/fmt/print.go:10: note",
			"~/go/pkg/mod/github.com/x/y@v1.2.3/z.go:4:2: undefined: q\n…/print.go:10: note",
		},
		{"In file included from /usr/include/stdio.h:64: `/opt/sdk/x.h` (-I/opt/include, PATH=/bin:/usr/bin)", "In file included from …/stdio.h:64: `…/x.h` (-I…/include, PATH=…/bin:…/bin)"},
		{"jsts project: TypeScript helper failed: <repository>/web/tsconfig.json: bad", "jsts project: TypeScript helper failed: web/tsconfig.json: bad"},
		{"two\tcolumns\r\nand /Users/someone/git/litestream-old/a.c and file:///etc/hosts", "two columns\nand ~/git/litestream-old/a.c and file://…/hosts"},
		{"a / b // c, x/y", "a / b // c, x/y"},
	} {
		got := FailureDetail(test.text, home, roots...)
		if got != test.want {
			t.Errorf("FailureDetail(%q)\n got %q\nwant %q", test.text, got, test.want)
		}
		if !ValidFailureDetail(got) {
			t.Errorf("FailureDetail(%q) = %q is not a valid detail", test.text, got)
		}
	}
	for _, detail := range []string{"", " padded", "open /etc/passwd", "a\tb", "<repository>/x"} {
		if ValidFailureDetail(detail) {
			t.Errorf("%q is a valid detail: it is empty, padded, holds a host path or a tab", detail)
		}
	}
}

// Whatever the error says, its detail is valid: a target's failure never
// becomes a failed run because its text could not be shown.
func TestFailureDetailOfAnyTextIsValidOrEmpty(t *testing.T) {
	pieces := []string{"/", "//", "/Users/someone", "/Users/someone/git/r", "<repository>", "~", "…", " ", "\n", "\t", "\x00", "\xff", ":", "'", "\"", "(", "-I", "file://", "a", "é", ".", "x/y", "@v1", "-"}
	random := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		var text strings.Builder
		for range random.IntN(12) {
			text.WriteString(pieces[random.IntN(len(pieces))])
		}
		got := FailureDetail(text.String(), "/Users/someone", "/Users/someone/git/r")
		if got != "" && !ValidFailureDetail(got) {
			t.Fatalf("FailureDetail(%q) = %q is not valid", text.String(), got)
		}
	}
}
