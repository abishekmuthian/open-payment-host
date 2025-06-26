package subscriptions

type PaypalCreateOrder struct {
	Intent             string             `json:"intent"`
	PaymentSource      PaymentSource      `json:"payment_source,omitzero"`
	ApplicationContext ApplicationContext `json:"application_context,omitzero"`
	PurchaseUnits      []PurchaseUnits    `json:"purchase_units,omitempty"`
}
type ExperienceContext struct {
	PaymentMethodPreference string `json:"payment_method_preference,omitempty"`
	LandingPage             string `json:"landing_page,omitempty"`
	ShippingPreference      string `json:"shipping_preference,omitempty"`
	UserAction              string `json:"user_action,omitempty"`
	ReturnURL               string `json:"return_url,omitempty"`
	CancelURL               string `json:"cancel_url,omitempty"`
}
type Paypal struct {
	ExperienceContext ExperienceContext `json:"experience_context,omitempty"`
}
type PaymentSource struct {
	Paypal Paypal `json:"paypal,omitzero"`
	Card   Card   `json:"card,omitzero"`
}
type ApplicationContext struct {
	ShippingPreference string `json:"shipping_preference,omitempty"`
}
type ItemTotal struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Value        string `json:"value,omitempty"`
}
type Shipping struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Value        string `json:"value,omitempty"`
}
type Handling struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Value        string `json:"value,omitempty"`
}
type TaxTotal struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Value        string `json:"value,omitempty"`
}
type Insurance struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Value        string `json:"value,omitempty"`
}
type ShippingDiscount struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Value        string `json:"value,omitempty"`
}
type Discount struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Value        string `json:"value,omitempty"`
}
type Breakdown struct {
	ItemTotal        ItemTotal        `json:"item_total,omitzero"`
	Shipping         Shipping         `json:"shipping,omitzero"`
	Handling         Handling         `json:"handling,omitzero"`
	TaxTotal         TaxTotal         `json:"tax_total,omitzero"`
	Insurance        Insurance        `json:"insurance,omitzero"`
	ShippingDiscount ShippingDiscount `json:"shipping_discount,omitzero"`
	Discount         Discount         `json:"discount,omitzero"`
}
type Amount struct {
	CurrencyCode string    `json:"currency_code,omitempty"`
	Value        string    `json:"value,omitempty"`
	Breakdown    Breakdown `json:"breakdown,omitzero"`
}
type UnitAmount struct {
	CurrencyCode string `json:"currency_code,omitempty"`
	Value        string `json:"value,omitempty"`
}
type Upc struct {
	Type string `json:"type,omitempty"`
	Code string `json:"code,omitempty"`
}
type Items struct {
	Name        string     `json:"name,omitempty"`
	Quantity    int        `json:"quantity,omitempty"`
	Description string     `json:"description,omitempty"`
	UnitAmount  UnitAmount `json:"unit_amount,omitempty"`
	Category    string     `json:"category,omitempty"`
	Sku         string     `json:"sku,omitempty"`
	ImageURL    string     `json:"image_url,omitempty"`
	URL         string     `json:"url,omitempty"`
	Upc         Upc        `json:"upc,omitzero"`
}
type PurchaseUnits struct {
	CustomID  string  `json:"custom_id,omitempty"`
	InvoiceID string  `json:"invoice_id,omitempty"`
	Amount    Amount  `json:"amount,omitempty"`
	Items     []Items `json:"items,omitempty"`
}

type Card struct {
	Name           string         `json:"name,omitempty"`
	Number         string         `json:"number,omitempty"`
	SecurityCode   string         `json:"security_code,omitempty"`
	Expiry         string         `json:"expiry,omitempty"`
	BillingAddress BillingAddress `json:"billing_address,omitzero"`
	Attributes     Attributes     `json:"attributes,omitempty"`
}

type BillingAddress struct {
	AddressLine1 string `json:"address_line_1,omitempty"`
	AddressLine2 string `json:"address_line_2,omitempty"`
	AdminArea2   string `json:"admin_area_2,omitempty"`
	AdminArea1   string `json:"admin_area_1,omitempty"`
	PostalCode   string `json:"postal_code,omitempty"`
	CountryCode  string `json:"country_code,omitempty"`
}

type Attributes struct {
	Customer Customer `json:"customer,omitzero"`
	Valult   Vault    `json:"vault,omitzero"`

	Verification Verification `json:"verification,omitempty"`
}

type Customer struct {
	ID                 string `json:"id,omitempty"`
	EmailAddress       string `json:"email_address,omitempty"`
	Phone              Phone  `json:"phone,omitzero"`
	Name               Name   `json:"name,omitzero"`
	MerchantCustomerID string `json:"merchant_customer_id,omitempty"`
}

type Phone struct {
	PhoneType   string `json:"phone_type,omitempty"`
	PhoneNumber struct {
		NationalNumber string `json:"national_number,omitempty"`
	} `json:"phone_number,omitempty"`
}

type Name struct {
	GivenName string `json:"given_name,omitempty"`
	Surname   string `json:"surname,omitempty"`
}

type Vault struct {
	StoreInVault string `json:"store_in_vault,omitempty"`
}

type Verification struct {
	Method string `json:"method,omitempty"`
}
