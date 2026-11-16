package products

import "github.com/abishekmuthian/open-payment-host/src/invoices"

func (s *Story) GST() invoices.Product {
	return invoices.Product{Enabled: s.GSTEnabled != 0, Code: s.GSTCode, Rate: s.GSTRate, Inclusive: s.GSTInclusive != 0, Export: s.GSTExport != 0}
}
