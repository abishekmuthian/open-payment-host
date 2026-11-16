package products

import _ "embed"

//go:embed views/countries.html.got
var countryOptionsHTML string

// CountryOptionsHTML keeps the pricing country list available while development
// template scans temporarily change the process working directory.
func CountryOptionsHTML() string {
	return countryOptionsHTML
}
