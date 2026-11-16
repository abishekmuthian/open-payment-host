package invoices

import (
	"strings"

	"github.com/signintech/gopdf"
)

// The text renderer starts a new page when a line would begin below this point.
const pdfLastLineY = 760.0
const pdfPageTextTop = 48.0

// wrappedPDFLines is shared by drawing and block measurement so wrapped names
// and long unbroken identifiers occupy exactly the space reserved for them.
func wrappedPDFLines(p *gopdf.GoPdf, width float64, value string, size int) ([]string, error) {
	if err := p.SetFont("invoice", "", size); err != nil {
		return nil, err
	}
	var lines []string
	for _, paragraph := range strings.Split(value, "\n") {
		line := ""
		flush := func() {
			lines = append(lines, line)
			line = ""
		}
		for _, word := range strings.Fields(paragraph) {
			candidate := word
			if line != "" {
				candidate = line + " " + word
			}
			w, err := p.MeasureTextWidth(candidate)
			if err != nil {
				return nil, err
			}
			if w <= width {
				line = candidate
				continue
			}
			if line != "" {
				flush()
			}
			for _, r := range word {
				candidate = line + string(r)
				w, err := p.MeasureTextWidth(candidate)
				if err != nil {
					return nil, err
				}
				if w > width && line != "" {
					flush()
				}
				line += string(r)
			}
		}
		flush()
	}
	return lines, nil
}
