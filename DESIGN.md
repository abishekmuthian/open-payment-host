# UI design conventions

Use these conventions for new and modified screens, including product forms,
payment gateway and API fields, invoices, and invoice settings. They capture the
design agreed for the existing forms. Agent workflow instructions live in
[AGENTS.md](AGENTS.md).

## Theme and components

- Use the existing Tailwind and DaisyUI components: `input`, `select`, `textarea`,
  `file-input`, `toggle`, `checkbox`, and `btn`.
- Follow the light and dark themes configured in
  [tailwind/tailwind.css](tailwind/tailwind.css). Use inherited or semantic theme
  colors instead of hardcoded foreground, background, or border colors.
- Use `btn btn-neutral` for a new form's main submit/save action. This gives the
  usual black action in the light theme and adapts to the dark theme. Do not use
  the blue `btn-primary` for ordinary submit/save actions. Use `self-start` when a
  button inside a column should retain its natural width.
- Use `btn` for secondary buttons and `link` for text links. Preserve the existing
  meaning and behavior of destructive actions.
- On product pages, put GST treatment inside the payment button as a smaller
  second line (`text-xs font-normal`), with the price as the main label. Apply
  this to all gateways and both GST-inclusive/exclusive pricing; avoid a separate
  GST paragraph above the button. Show the treatment only for domestic GST sales.
- Build interactions with the project's server-rendered Go templates, HTMX, and
  Hyperscript. Reuse the existing asset pipeline and gateway SDK integrations.

## Forms and spacing

The Product Summary group in the
[product create form](src/products/views/create.html.got) is the spacing reference.

| Relationship | Convention |
| --- | --- |
| Label, description, and control within a field group | `flex flex-col gap-3` (12px) |
| Between separate field groups or repeated pricing rows | `flex flex-col gap-5` (20px) |
| Field label | `block text-sm/6 font-medium` with `label-text text-xl` on its text |
| Label containing its control | `flex flex-col gap-3` with `label-text text-xl font-medium` on its text |
| Description/help text | `text-sm/6` |
| Control width | `w-full` with the existing appropriate `max-w-*` limit |
| Hidden validation hint | `validator-hint hidden`, paired with a `validator` control |

Prefer container gaps over accumulating child margins or nested `space-y-*`
utilities, especially around conditional fields. A hidden hint must occupy no
space; DaisyUI reveals the hint when its associated control becomes invalid.
Do not tighten the label-to-control gap to compensate for an empty hint.

Associate every label with its control using `for`/`id` or a containing label.
Keep explanatory text next to the field it describes.

Billing forms must style optional business-name/GSTIN fields like the surrounding
fields, including label size/weight, neutral input borders, width and spacing.
Show both fields only for Indian billing addresses. For a foreign or unselected
country, hide the group, disable its inputs, and clear its values on country
changes so stale details cannot be submitted.
Country controls show full country names and submit their ISO codes; buyers
should not need to type codes. Razorpay's international country selector follows
State, with the detected country preselected where available.

For example:

```html
<div class="flex flex-col gap-5">
  <div class="flex flex-col gap-3">
    <label for="gst_code" class="block text-sm/6 font-medium">
      <span class="label-text text-xl">HSN/SAC code</span>
    </label>
    <input id="gst_code" name="gst_code" type="text"
      class="input validator w-full max-w-lg prose lg:prose-xl"
      pattern="[0-9]{4}([0-9]{2})?([0-9]{2})?" required />
    <p class="validator-hint hidden">Enter a 4, 6 or 8 digit HSN/SAC code.</p>
  </div>
  <!-- Further field groups use the same internal gap. -->
</div>
```

See the [GST fields](src/products/views/gst_fields.html.got),
[API fields](src/products/views/api_toggle.html.got), and
[invoice settings](src/invoices/views/settings.html.got) for existing examples.
Apply the same spacing to create, update, and dynamically returned partials.

## Conditional sections and payment flow

- Keep each section's label and enable toggle visible. Render its dependent
  controls only when enabled, following the payment gateway HTMX toggle pattern.
  Disabling the section must clear its target, removing required controls from
  the DOM so they cannot block submission.
- Create forms start with optional sections disabled. Update forms restore saved
  toggle state and load saved fields by triggering `change` on page load.
  Toggle targets start empty in both forms.
- Place GST invoicing at the bottom of the product create/update form,
  immediately above Submit/Update, after the payment gateway and API sections.
- Use declarative HTMX requests and Hyperscript events. Toggle rendering endpoints
  must retain CSRF and authorization checks and return partials without the page
  layout. Follow the endpoint guidance in [AGENTS.md](AGENTS.md).
- Dependent updates must work regardless of which section loads first. For GST
  pricing, retain the `gstPricingChanged` event on `body`, including the event
  after the GST partial is swapped; avoid binding only to fields absent at load.
- Reuse existing billing collection. Keep invoice details inline where needed;
  do not add an invoice preview or confirmation step before payment.

The [GST toggle](src/products/views/gst.html.got) demonstrates the conditional
section pattern. Preserve stored values, field names, request parameters, and
schedule-specific behavior when changing presentation.

## Responsive gateway pricing rows

Stack pricing controls on mobile. From the `sm` breakpoint (640px), keep the
country, amount/plan, tax, currency, and remove button on the same row. Controls
must shrink within the available width so the remove button stays beside them.

| Element | Layout classes |
| --- | --- |
| Pricing row | `flex flex-col gap-3 sm:flex-row sm:flex-nowrap sm:items-start` |
| Country select | `select min-w-0 sm:flex-[2_1_0%] w-full max-w-60 rounded-sm` |
| Amount, plan, or currency input | `input min-w-0 sm:flex-1 w-full` plus the existing width limit |
| Tax HTMX wrapper | `w-full min-w-0 max-w-24 sm:flex-1` |
| Remove button wrapper | `flex shrink-0` |
| Label wrapping a pricing control | `flex min-w-0 flex-col gap-3 sm:flex-1` |

Constrain wrappers as well as their inputs: a tax input replaced by HTMX must
remain inside a flexible wrapper. Use `min-w-0 w-full` on an input inside a
flexible label. Avoid `sm:flex-wrap` on these rows because it sends the remove
button to a second line.

See the [PayPal pricing row](src/products/views/paypal_price_onetime.html.got)
and shared [country select](src/products/views/countries.html.got). Apply the same
layout to Stripe, Square, PayPal, and Razorpay, including initial rows, added
countries, saved update rows, and all supported schedules. Preserve field IDs,
indices, tax calculations, and add/remove behavior.

## Invoice PDFs

- Put **Bill to:** on its own line, with the customer's name, business and
  address below it.
- Use item-table columns for **Description**, **HSN/SAC**, and **Amount**. Keep
  the product's code beside its description, with enough room for an eight-digit
  code and wrapping descriptions. Keep place of supply in the billing section.
- Omit product-title hashtags using the existing product display convention.
- Label the export reference **LUT ARN** in both invoice settings and newly
  generated invoice/credit-note PDFs.
- Display the LUT financial year as `2026-27`, matching the invoice settings form;
  compact values such as `2627` are an internal storage format.
- Keep the signature image and wrapped signatory caption together. Measure the
  actual scaled image height, a 10-point gap, and caption line count before
  choosing a page break; do not reserve a fixed signature height or use a fixed
  footer cutoff. Preserve the 130-point width and 55-point height limits. Move
  the whole block only when its final text line would start below 760 points.
- Keep the reverse-charge declaration. The current forward-charge/LUT workflow
  prints **Reverse charge: No**; reverse-charge supplies need separate support.
- Verify synthetic domestic, export and credit-note PDFs, including long text,
  by rendering them to images. Do not rewrite issued PDFs for layout changes.

## Building and checking changes

Edit template classes or Tailwind source; generate the committed stylesheet
instead of editing the minified output by hand:

```sh
npx @tailwindcss/cli -i ./tailwind/tailwind.css -o ./src/app/assets/styles/app.css --minify
```

For UI changes, check the affected states in a browser:

- Light and dark themes, labels, descriptions, focus, and validation hints.
- Create and update forms, sections enabled/disabled, and saved values.
- Pricing rows before and after adding/removing a country, for one-time,
  monthly, and yearly schedules across all affected gateways.
- Mobile at 375px and responsive widths at 640px, 768px, 1024px, and 1440px;
  confirm no horizontal overflow and that remove buttons stay inline from `sm`.
- HTMX swaps retain spacing, labels, and controls; disabled sections do not
  leave blank space or required fields behind.

Run existing checks appropriate to behavior changes. HTTP and unit tests do not
establish visual layout; use browser verification for geometry changes. A
documentation-only update needs link and whitespace checks, without rebuilding
CSS or rerunning application tests.
