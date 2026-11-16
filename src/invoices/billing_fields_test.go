package invoices

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

func TestBillingFieldsOnlyEnableBusinessAndGSTINForIndia(t *testing.T) {
	testenv.Config(t, nil)
	testenv.Templates(t)
	for _, country := range []string{"IN", "US", ""} {
		t.Run(country, func(t *testing.T) {
			v := view.NewWithPath("", nil)
			v.Layout("")
			v.Template("invoices/views/billing_fields.html.got")
			v.AddKey("billing", Billing{Country: country, Business: "Example Business", GSTIN: "33AAAAA0000A1Z9"})
			body, err := v.RenderToStringWithLayout()
			if err != nil {
				t.Fatal(err)
			}
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"business", "gstin"} {
				field := doc.Find("[name='billing_" + name + "']")
				_, disabled := field.Attr("disabled")
				if field.Length() != 1 || disabled != (country != "IN") || (field.ParentsFiltered("[hidden]").Length() > 0) != (country != "IN") {
					t.Fatal("business name and GSTIN are not restricted to Indian billing")
				}
				if country != "IN" && field.AttrOr("value", "") != "" {
					t.Fatal("hidden international billing fields retained values")
				}
			}
		})
	}
}
