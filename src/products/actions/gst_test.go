package storyactions

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/invoices"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

func TestGSTSectionAtBottomWithEmptyToggleTarget(t *testing.T) {
	f := setupHandlers(t, nil)
	id := testenv.SeedProduct(t, map[string]interface{}{"user_id": f.admin, "gst_enabled": 1, "gst_code": "998439", "gst_rate": "18"})
	for _, path := range []string{"/products/create", fmt.Sprintf("/products/%d/update", id)} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		testenv.LoggedIn(t, request, f.admin)
		w := testenv.Serve(t, request)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(w.Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		section := doc.Find("#gst-invoicing")
		if section.Length() != 1 || section.Next().Find("button[type='submit']").Length() != 1 {
			t.Fatalf("%s: GST section must immediately precede submit container", path)
		}
		if doc.Find("#gst_code, #gst_rate, #gst_inclusive, #gst_export").Length() != 0 || strings.TrimSpace(doc.Find("#gst-details").Text()) != "" {
			t.Fatalf("%s: fields must be loaded only by an enabled toggle", path)
		}
		toggle := section.Find("#gst_enabled")
		_, checked := toggle.Attr("checked")
		if checked != (path != "/products/create") {
			t.Fatalf("%s: incorrect saved toggle state", path)
		}
		if endpoint, _ := toggle.Attr("hx-post"); endpoint == "" {
			t.Fatal("GST toggle lacks HTMX endpoint")
		}
	}
}

func TestGSTToggleLoadsDefaultsSavedValuesAndClearsFields(t *testing.T) {
	f := setupHandlers(t, nil)
	id := testenv.SeedProduct(t, map[string]interface{}{"user_id": f.reader, "gst_enabled": 1, "gst_code": "998439", "gst_rate": "18", "gst_inclusive": 0, "gst_export": 1})
	for _, tc := range []struct {
		path   string
		user   int64
		update bool
	}{{"/products/toggle/gst", f.admin, false}, {fmt.Sprintf("/products/%d/toggle/gst", id), f.reader, true}} {
		on := testenv.Serve(t, testenv.AuthedPOST(t, tc.path, url.Values{"gst_enabled": {"on"}}, tc.user))
		if on.Code != http.StatusOK || strings.Contains(on.Body.String(), "<html") {
			t.Fatalf("toggle partial: %d %s", on.Code, on.Body.String())
		}
		if on.Header().Get("HX-Trigger-After-Swap") != "gstPricingChanged" {
			t.Fatal("PayPal tax preview must refresh after fields are swapped")
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(on.Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"gst_code", "gst_rate"} {
			if _, ok := doc.Find("#" + field).Attr("required"); !ok {
				t.Fatalf("enabled %s must be required", field)
			}
		}
		_, inclusive := doc.Find("#gst_inclusive").Attr("checked")
		if inclusive == tc.update {
			t.Fatal("create defaults or saved exclusive mode not respected")
		}
		if tc.update {
			if code, _ := doc.Find("#gst_code").Attr("value"); code != "998439" {
				t.Fatal("saved HSN/SAC not restored")
			}
			if rate, _ := doc.Find("#gst_rate").Attr("value"); rate != "18" {
				t.Fatal("saved rate not restored")
			}
			if _, export := doc.Find("#gst_export").Attr("checked"); !export {
				t.Fatal("saved export flag not restored")
			}
		}
		if !doc.Find("#gst_code").HasClass("validator") || !doc.Find("#gst_export").HasClass("toggle") {
			t.Fatal("GST controls must follow existing themed form classes")
		}
		off := testenv.Serve(t, testenv.AuthedPOST(t, tc.path, nil, tc.user))
		if off.Code != http.StatusOK || !testenv.Blank(off.Body.String()) {
			t.Fatal("disabled GST toggle must clear all dependent fields")
		}
	}
}

func TestGSTTogglesRequireCSRFAndProductAuthorisation(t *testing.T) {
	f := setupHandlers(t, nil)
	id := testenv.SeedProduct(t, map[string]interface{}{"user_id": f.admin, "gst_enabled": 1, "gst_code": "998439"})
	for _, path := range []string{"/products/toggle/gst", fmt.Sprintf("/products/%d/toggle/gst", id)} {
		for _, values := range []url.Values{nil, {"gst_enabled": {"on"}}} {
			if w := testenv.Serve(t, testenv.AuthedPOST(t, path, values, 0)); w.Code != 401 {
				t.Fatal("anonymous toggle allowed", path)
			}
			if w := testenv.Serve(t, testenv.POST(path, values)); w.Code != 401 {
				t.Fatal("CSRF-less toggle allowed", path)
			}
		}
	}
	path := fmt.Sprintf("/products/%d/toggle/gst", id)
	if w := testenv.Serve(t, testenv.AuthedPOST(t, path, url.Values{"gst_enabled": {"on"}}, f.reader)); w.Code != 401 {
		t.Fatal("non-owner can read GST settings")
	}
}

func TestEnableGSTRequiresSavedSellerSettings(t *testing.T) {
	f := setupHandlers(t, nil)
	id := testenv.SeedProduct(t, map[string]interface{}{"user_id": f.admin})
	paths := []string{"/products/create", fmt.Sprintf("/products/%d/update", id)}
	form := url.Values{
		"name": {"GST handbook"}, "summary": {"Summary"}, "description": {"<p>Body</p>"}, "schedule": {"onetime"}, "status": {"100"},
		"gst_enabled": {"on"}, "gst_code": {"998439"}, "gst_rate": {"18"}, "gst_inclusive": {"on"},
	}
	// Capture the same user-facing error metadata used by the production renderer.
	router := testenv.Router()
	prior := router.ErrorHandler
	var failure *server.StatusError
	router.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		failure = server.ToStatusError(err)
		prior(w, r, err)
	}
	t.Cleanup(func() { router.ErrorHandler = prior })
	assertRejected := func(title string, status int) {
		t.Helper()
		for _, path := range paths {
			failure = nil
			w := testenv.Serve(t, testenv.AuthedPOST(t, path, form, f.admin))
			if w.Code != status || failure == nil || failure.Title != title {
				t.Fatalf("%s: expected %d %q, got %d %+v", path, status, title, w.Code, failure)
			}
			if status == 400 && !strings.Contains(failure.Message, "/admin/invoicing") {
				t.Fatal("seller setup error has no actionable settings destination")
			}
			p, err := products.Find(id)
			if err != nil || p.GSTEnabled != 0 || p.Name != "Test product" || testenv.Scalar(t, "SELECT COUNT(*) FROM products") != 1 {
				t.Fatal("rejected GST operation changed an existing product or created a new one", err)
			}
		}
	}
	assertRejected("Set up invoicing first", 400)
	for _, path := range []string{"/products/toggle/gst", fmt.Sprintf("/products/%d/toggle/gst", id)} {
		w := testenv.Serve(t, testenv.AuthedPOST(t, path, url.Values{"gst_enabled": {"on"}}, f.admin))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "before enabling GST invoicing") || !strings.Contains(w.Body.String(), `href="/admin/invoicing"`) {
			t.Fatal("GST toggle lacks seller setup notice", w.Code)
		}
	}
	if _, err := gstParams(map[string]string{}); err != nil {
		t.Fatal("missing invoicing profile blocked a non-GST product", err)
	}
	testenv.Exec(t, "INSERT INTO invoice_settings(id,value) VALUES(1,?)", invoices.JSON(invoices.Settings{}))
	assertRejected("Complete invoicing settings", 400)
	testenv.Exec(t, "UPDATE invoice_settings SET value='corrupt'")
	assertRejected("Unable to load invoicing settings", 500)
	seller := invoices.Settings{Name: "Seller", Address: "Coimbatore", GSTIN: "33AAAAA0000A1Z9", State: "33", Email: "seller@example.test", Signatory: "Seller", Prefix: "OPH", CreditPrefix: "CN", Start: 1}
	testenv.Exec(t, "UPDATE invoice_settings SET value=?", invoices.JSON(seller))
	for _, path := range paths {
		w := testenv.Serve(t, testenv.AuthedPOST(t, path, form, f.admin))
		if w.Code != http.StatusFound {
			t.Fatalf("GST product save failed after seller setup: %s %d %s", path, w.Code, w.Body.String())
		}
	}
	p, err := products.Find(id)
	if err != nil || p.GSTEnabled != 1 || p.GSTCode != "998439" || p.GSTRate != "18" || p.GSTInclusive != 1 || testenv.Scalar(t, "SELECT COUNT(*) FROM products WHERE gst_enabled=1") != 2 {
		t.Fatal("GST settings not saved on the existing and new products", err)
	}
	w := testenv.Serve(t, testenv.AuthedPOST(t, fmt.Sprintf("/products/%d/toggle/gst", id), url.Values{"gst_enabled": {"on"}}, f.admin))
	if w.Code != 200 || strings.Contains(w.Body.String(), "before enabling GST invoicing") {
		t.Fatal("seller setup notice remains after valid settings were saved")
	}
}
