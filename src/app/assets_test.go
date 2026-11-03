package app

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// staleManifest is a secrets/assets.json from before the vendored libraries
// were added: it knows app.js only, not htmx.js.
const staleManifest = `{"app":{"scripts":"0000","styles":"1111","files":{"src/app/assets/scripts/app.js":"2222","src/app/assets/styles/app.css":"3333"}}}`

// vendoredAssets are bundled in the app group and must never get their own tags.
var vendoredAssets = []string{"htmx.js", "hyperscript.js", "sweetalert2.js", "trix.js", "trix.css"}

// TestAssetsInEveryMode runs SetupAssets for both assets_compiled values in
// development and production, starting from a stale manifest each time.
func TestAssetsInEveryMode(t *testing.T) {
	for _, production := range []bool{false, true} {
		for _, compiled := range []string{"yes", "no"} {
			t.Run(fmt.Sprintf("production=%v/compiled=%s", production, compiled), func(t *testing.T) {
				testAssetsMode(t, production, compiled == "yes")
			})
		}
	}
}

func testAssetsMode(t *testing.T, production, compiled bool) {
	setupApp(t)
	savedAssets, savedStyle, savedScript := appAssets, view.Helpers["style"], view.Helpers["script"]
	t.Cleanup(func() {
		appAssets, view.Helpers["style"], view.Helpers["script"] = savedAssets, savedStyle, savedScript
	})

	if production {
		testenv.Production(t)
	}
	if compiled {
		testenv.Set("assets_compiled", "yes")
	} else {
		testenv.Set("assets_compiled", "no")
	}
	if err := os.WriteFile(filepath.Join("secrets", "assets.json"), []byte(staleManifest), 0644); err != nil {
		t.Fatal(err)
	}

	SetupAssets()

	for _, name := range vendoredAssets {
		folder := "scripts"
		if strings.HasSuffix(name, ".css") {
			folder = "styles"
		}
		p := "/assets/" + folder + "/" + name
		w := serve(t, httptest.NewRequest(http.MethodGet, p, nil))
		if ct := w.Header().Get("Content-Type"); w.Code != http.StatusOK || strings.Contains(ct, "text/html") {
			t.Errorf("%s: status=%d content-type=%q", p, w.Code, ct)
		}
	}

	head := renderHead(t)
	scripts := regexp.MustCompile(`/assets/scripts/app-[0-9a-f]+\.min\.js`).FindAllString(head, -1)
	styles := regexp.MustCompile(`/assets/styles/app-[0-9a-f]+\.min\.css`).FindAllString(head, -1)

	if compiled {
		if len(scripts) != 1 || len(styles) != 1 {
			t.Fatalf("want one bundle script and stylesheet, got %v %v in:\n%s", scripts, styles, head)
		}
		for _, name := range vendoredAssets {
			if strings.Contains(head, "/"+name) {
				t.Errorf("compiled head has a separate %s tag:\n%s", name, head)
			}
		}
		for _, p := range append(scripts, styles...) {
			w := serve(t, httptest.NewRequest(http.MethodGet, p, nil))
			if w.Code != http.StatusOK || w.Body.Len() == 0 {
				t.Errorf("%s: status=%d size=%d", p, w.Code, w.Body.Len())
			}
			if strings.HasSuffix(p, ".js") && !strings.Contains(w.Body.String(), "htmx") {
				t.Errorf("%s does not contain htmx", p)
			}
		}
		return
	}

	if len(scripts) != 0 || len(styles) != 0 {
		t.Errorf("separate mode linked bundles %v %v", scripts, styles)
	}
	for _, name := range vendoredAssets {
		if n := strings.Count(head, "/"+name+`"`); n != 1 {
			t.Errorf("separate head links %s %d times, want once:\n%s", name, n, head)
		}
	}
}

// renderHead renders the layout's asset templates with the helpers that
// SetupAssets just installed. Loaded templates captured the helpers at parse
// time, so they are parsed again here. Every page flag is set, so no flag may
// add a duplicate tag.
func renderHead(t *testing.T) string {
	t.Helper()
	funcs := template.FuncMap{}
	for k, v := range view.Helpers {
		funcs[k] = v
	}
	var b strings.Builder
	for _, name := range []string{"scripts.html.got", "includes.html.got"} {
		body, err := os.ReadFile(filepath.Join("src", "app", "views", name))
		if err != nil {
			t.Fatal(err)
		}
		tmpl, err := template.New(name).Funcs(funcs).Parse(string(body))
		if err != nil {
			t.Fatal(err)
		}
		data := map[string]interface{}{"loadTrixScript": true, "loadHypermedia": true, "loadSweetAlert": true}
		if err := tmpl.Execute(&b, data); err != nil {
			t.Fatal(err)
		}
	}
	return b.String()
}
