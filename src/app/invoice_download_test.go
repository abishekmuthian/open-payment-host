package app

import (
	"bytes"
	"compress/gzip"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/invoices"
)

func TestInvoicePDFDownloadThroughApplicationRoutes(t *testing.T) {
	setupApp(t)
	pdf := []byte("%PDF-1.4\nsynthetic persisted document\n%%EOF\n")
	readerID := testenv.SeedUser(t, 20, "reader@example.test", "test-reader-password")
	for _, kind := range []string{"invoice", "credit_note"} {
		id, number := "download-"+kind, "2026-TEST-0017"
		if kind == "credit_note" {
			id, number = "download-credit-note", "2026-CN-0017"
		}
		s := invoices.Snapshot{Currency: "INR", Gross: 11800}
		testenv.Exec(t, "INSERT INTO invoices(id,kind,gateway,transaction_id,attempt_id,created_at,issued_at,number,snapshot,status,pdf) VALUES(?,?,?,?,?,?,?,?,?,'issued',?)", id, kind, "razorpay", id, "test-attempt", "2026-11-15", "2026-11-15", number, invoices.JSON(s), pdf)
		path := "/admin/invoices/" + id
		for _, encoding := range []string{"identity", "gzip"} {
			t.Run(kind+"/"+encoding, func(t *testing.T) {
				r := httptest.NewRequest(http.MethodGet, path+"/download", nil)
				r.Header.Set("Accept-Encoding", encoding)
				testenv.LoggedIn(t, r, 1)
				w := serve(t, r)
				if w.Code != 200 || w.Header().Get("Content-Type") != "application/pdf" {
					t.Fatalf("download returned status %d, type %q instead of a PDF", w.Code, w.Header().Get("Content-Type"))
				}
				disposition, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
				if err != nil || disposition != "attachment" || params["filename"] != number+".pdf" {
					t.Fatal("download attachment headers are incorrect", err)
				}
				if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal("private download headers are missing")
				}
				body := w.Body.Bytes()
				if encoding == "gzip" {
					reader, err := gzip.NewReader(bytes.NewReader(body))
					if err != nil {
						t.Fatal(err)
					}
					defer reader.Close()
					body, err = io.ReadAll(reader)
					if err != nil {
						t.Fatal(err)
					}
				}
				if !bytes.Equal(body, pdf) {
					t.Fatal("download did not preserve the stored PDF bytes")
				}
			})
		}
		for _, userID := range []int64{0, readerID} {
			r := httptest.NewRequest(http.MethodGet, path+"/download", nil)
			if userID != 0 {
				testenv.LoggedIn(t, r, userID)
			}
			if w := serve(t, r); w.Code != 401 || bytes.Contains(w.Body.Bytes(), pdf) {
				t.Fatal("non-admin could download the document")
			}
		}
		r := httptest.NewRequest(http.MethodGet, path, nil)
		testenv.LoggedIn(t, r, 1)
		if w := serve(t, r); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Download PDF")) {
			t.Fatal("document page no longer renders its download link")
		}
	}
}
