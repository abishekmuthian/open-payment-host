package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testCSS = `.a{width:calc(69% + 0.25px);margin:calc(1rem - 2px)}
select{&::picker(select){appearance:base-select}}
@layer base{.prose :where(p){margin:0}}
`
	testScriptA = "const a = `x ${1 + 2}`;\nlet b = a?.length ?? 0;\n"
	testScriptB = "class C { #p = 1; static s = () => /re+gex/.test('e'); }\n"
)

// compileFixture compiles a small src tree into a temporary directory and
// returns the collection and the destination root.
func compileFixture(t *testing.T) (*Collection, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	files := map[string]string{
		"src/app/assets/styles/app.css":        testCSS,
		"src/app/assets/scripts/a.js":          testScriptA,
		"src/app/assets/scripts/b.js":          testScriptB,
		"public/assets/scripts/app-old.min.js": "stale",
	}
	for p, body := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"data/secrets", "public/assets/styles"} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	c := New(true)
	if err := c.Compile("src", "public"); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c, "public"
}

func TestCompilePreservesModernCSS(t *testing.T) {
	c, dst := compileFixture(t)
	out, err := os.ReadFile(c.Group("app").StylePath(dst))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"calc(69% + 0.25px)", "calc(1rem - 2px)", "&::picker(select)", "@layer base", ".prose :where(p)"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("compiled CSS lost %q:\n%s", want, out)
		}
	}
}

func TestCompileScriptBundle(t *testing.T) {
	c, dst := compileFixture(t)
	g := c.Group("app")
	out, err := os.ReadFile(g.ScriptPath(dst))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("compiled JS bundle is empty")
	}
	bundle := string(out)
	ia, ib := strings.Index(bundle, testScriptA), strings.Index(bundle, testScriptB)
	if ia < 0 || ib < 0 {
		t.Fatalf("bundle does not contain every source file unchanged:\n%s", bundle)
	}
	if ia > ib {
		t.Error("bundle is not in filename order")
	}
	if _, err := os.Stat(filepath.Join(dst, "assets", "scripts", "app-old.min.js")); !os.IsNotExist(err) {
		t.Error("old compiled bundle was not removed")
	}
}

func TestLinksForCompiledAndSeparateModes(t *testing.T) {
	c, _ := compileFixture(t)
	g := c.Group("app")

	compiled := string(c.ScriptLink("app"))
	if strings.Count(compiled, "<script") != 1 || !strings.Contains(compiled, g.ScriptName()) {
		t.Errorf("compiled mode should emit one bundle tag, got %s", compiled)
	}
	if css := string(c.StyleLink("app")); strings.Count(css, "<link") != 1 || !strings.Contains(css, g.StyleName()) {
		t.Errorf("compiled mode should emit one stylesheet tag, got %s", css)
	}

	c.serveCompiled = false
	separate := string(c.ScriptLink("app"))
	if strings.Count(separate, "<script") != 2 || !strings.Contains(separate, "a.js") || !strings.Contains(separate, "b.js") {
		t.Errorf("separate mode should emit one tag per file, got %s", separate)
	}
}
