package helpers

import (
	"fmt"

	"runtime"
	"time"

	//"strings"
	"testing"
)

// Enable db debug mode to enable query logging
var debug = "false"

var Format = "\n---\nFAILURE\n---\ninput:    %q\nexpected: %q\noutput:   %q"

type Test struct {
	input    string
	expected string
}

func testLog() {
	pc, _, line, _ := runtime.Caller(1)
	name := runtime.FuncForPC(pc).Name()
	fmt.Printf("TEST LOG line %d - %s\n", line, name)
}

// TestPrices tests price conversion
func TestPrices(t *testing.T) {
	fmt.Println("\n---\nTESTING Prices\n---")

	var pence int
	var price string

	price = "£10.00"
	pence = PriceToCents(price)
	if pence != 1000 {
		t.Fatalf(Format, price, "1000", fmt.Sprintf("%d", pence))
	}
	price = CentsToPrice(int64(pence))
	if price != "£10" {
		t.Fatalf(Format, price, "£10", fmt.Sprintf("%d", pence))
	}

	price = "10"
	pence = PriceToCents(price)
	if pence != 1000 {
		t.Fatalf(Format, price, "1000", fmt.Sprintf("%d", pence))
	}
	price = CentsToPrice(int64(pence))
	if price != "£10" {
		t.Fatalf(Format, price, "£10", fmt.Sprintf("%d", pence))
	}

	price = "45"
	pence = PriceToCents(price)
	if pence != 4500 {
		t.Fatalf(Format, price, "4500", fmt.Sprintf("%d", pence))
	}
	price = CentsToPrice(int64(pence))
	if price != "£45" {
		t.Fatalf(Format, price, "45.00", fmt.Sprintf("%d", pence))
	}

	price = "45.35"
	pence = PriceToCents(price)
	if pence != 4535 {
		t.Fatalf(Format, price, "4535", fmt.Sprintf("%d", pence))
	}

	price = CentsToPrice(int64(pence))
	if price != "£45.35" {
		t.Fatalf(Format, price, "45.35", fmt.Sprintf("%d", pence))
	}

	price = "45.30"
	pence = PriceToCents(price)
	if pence != 4530 {
		t.Fatalf(Format, price, "4530", fmt.Sprintf("%d", pence))
	}

	price = CentsToPrice(int64(pence))
	if price != "£45.30" {
		t.Fatalf(Format, price, "45.30", fmt.Sprintf("%d", pence))
	}

}

var commaNumbers = map[int64]string{
	100:       "100",
	1000:      "1,000",
	102001:    "102,001",
	31300002:  "31,300,002",
	12001:     "12,001",
	300002:    "300,002",
	74002:     "74,002",
	450000003: "450,000,003",
}

func TestNumberToCommas(t *testing.T) {
	for k, v := range commaNumbers {
		r := NumberToCommas(k)
		if r != v {
			t.Errorf("numbertocommas: wanted:%s got:%s", v, r)
		}
	}
}

// TestDateTimeLayoutPercent checks that a percent sign in a layout is not
// treated as a format verb.
func TestDateTimeLayoutPercent(t *testing.T) {
	when := time.Date(2026, 9, 29, 14, 5, 0, 0, time.UTC)

	if got, want := string(Date(when, "Jan 2006 %d")), "Sep 2026 %d"; got != want {
		t.Fatalf(Format, "Date", want, got)
	}
	if got, want := string(Time(when, "15:04 %")), "14:05 %"; got != want {
		t.Fatalf(Format, "Time", want, got)
	}
}
