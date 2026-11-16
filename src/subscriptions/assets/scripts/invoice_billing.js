// Persist invoice billing before opening a provider modal. This never sends a
// charge amount: the server alone resolves pricing and payment expectations.
async function saveInvoiceBilling() {
  const root = document.querySelector('[data-invoice-attempt]');
  if (!root) return;
  const invalid = Array.from(root.querySelectorAll('input,select,textarea')).find(field => !field.checkValidity());
  if (invalid) { invalid.reportValidity(); const error = new Error('Please complete your billing details.'); error.invoiceBilling = true; throw error; }
  const value = (name, fallback) => {
    const field = root.querySelector('[name="billing_' + name + '"]') || document.querySelector(fallback || '[data-no-field]');
    return field && !field.disabled ? field.value.trim() : '';
  };
  const country = value('country');
  const domestic = country === 'IN';
  const data = new URLSearchParams({
    authenticity_token: authenticityToken(), attempt_id: root.dataset.invoiceAttempt,
    billing_name: value('name', '.razorpay-input-name'),
    billing_business: domestic ? value('business') : '', billing_email: value('email', '.razorpay-input-email'),
    billing_address: value('address', '.razorpay-input-address'), billing_city: value('city', '.razorpay-input-city'),
    billing_state: value('state', '.razorpay-input-state'), billing_postcode: value('postcode', '.razorpay-input-pincode'),
    billing_country: country, billing_gstin: domestic ? value('gstin') : ''
  });
  const response = await fetch('/subscriptions/invoice-billing', {method: 'POST', headers: {'Content-Type': 'application/x-www-form-urlencoded'}, body: data});
  if (!response.ok) { const error = new Error('Please check your invoice billing details, including state and GSTIN. Start a new checkout if previously submitted details have changed.'); error.invoiceBilling = true; throw error; }
}
