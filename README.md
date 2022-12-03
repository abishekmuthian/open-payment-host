# Open Payment Host

# Open Payment Host

Sell Subscriptions, Newsletters, Digital Files without paying commissions.

## What
Open Payment Host is an easy to run self-hosted, minimalist Payments Host through which we can easily sell our digital items without paying double commissions while having total control over our sales and data.

## Why
Selling digital items on web as an indie now requires using platforms where we have to pay double commissions(to the platform and the payment gateway) and our content is forever locked within those platforms.

## How
Open Payment Host is a minimalist yet highly performant Go web application with innovative features which helps indies self-host and sell digital items with little effort.

## Demo

## Features
* Customers can buy without logging in, Increases conversion.
* Stripe support, Just add the price id for the product and rest is done automatically.
* Multi-country pricing, Price changes automatically according to the user's location resulting in better conversion.
* Mailchimp support, Customers are automatically added to a mailchimp list; Useful for sending newsletters.
* WYSIWYG editor to create beautiful product pages.
* Automatic SSL and other security features for production.

More features are being added every day.

## Usage
<hr>

### Requirements
1. [Stripe](https://stripe.com/) account for payment gateway.
2. [Cloudflare](https://www.cloudflare.com/) account for turnstile captcha.
3. [Mailchimp](https://mailchimp.com/) account for adding subscribers to the list.

Note: Open Payment Host can be tested without fulfilling above requirements, But payments and adding subscribers to the list wouldn't work.

### Docker
The latest image is available on DockerHub at [`abishekmuthian/open-payment-host:latest`](https://hub.docker.com/layers/abishekmuthian/open-payment-host/latest).

The latest slimmed image is available on DockerHub at [`abishekmuthian/open-payment-host:latest-slim`](https://hub.docker.com/layers/abishekmuthian/open-payment-host/latest-slim)

#### Slim.AI container page


#### Demo

```bash
mkdir oph-demo && cd oph-demo
sh -c "$(curl -fsSL https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-demo/install-demo.sh)"
```

Visit `http://localhost:3000`.

DO NOT use this demo setup in production.

#### Production

It's recommended to try the demo application first before using the production application. The production application requires special config variables detailed in the configuration section.

```bash
mkdir oph-production && cd oph-production
sh -c "$(curl -fsSL https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-production/install-production.sh)"
```
Visit `http://localhost:443`.



