# Open Payment Host

Sell Subscriptions, Newsletters, Digital Files without paying commissions.

![Open Payment Host post page](/demo/Square/4.post.gif)

## What

Open Payment Host is an easy to run self-hosted, minimalist payments host through which we can easily sell our digital items without paying double commissions while having total control over our sales and data.

## Why

Selling digital items on web as an indie requires using platforms where we have to pay double commissions(to the platform and the payment gateway) and our content is forever locked within those platforms.

## How

Open Payment Host is a minimalist yet highly performant Go web application with innovative features which helps indies self-host and sell digital items with little effort.

## Video Demo

[![Video Demo](/demo/Stripe/thumbnail-site.png)](https://www.youtube.com/watch?v=vQcLr-NgqIU)
Clicking the above image would open the video in YouTube.

## Demo

[https://demo.openpaymenthost.com](https://demo.openpaymenthost.com)

## Features

- Customers can buy without logging in, Increases conversion.
- Stripe support, Just add the price id for the product and rest is done automatically.
- Square support, Just add the amount for the product and rest is done automatically.<sup>New</sup>
- Multi-country pricing, Price changes automatically according to the user's location resulting in better conversion.
- Mailchimp support, Customers are automatically added to a mailchimp list; Useful for sending newsletters.
- WYSIWYG editor with AI autocomplete to create beautiful product pages.<sup>New</sup>
- File attachment support(images) for the product posts. <sup>New</sup>
- S3 support for delivering digital files via automatic pre-signed URL. <sup>New</sup>
- Subscriber count for the products (With Square).<sup>New</sup>
- Automatic SSL and other security features for production.

and many more.

## Screenshots

### Home

![Hope page of OPH](/demo/Square/1.a.home.png)

### WYSIWYG editor with autocomplete powered by Google PaLM AI

![WYSIWYG editor](/demo/Square/10.editor-oph.gif)

### Buy without login

#### Square

![Billing](/demo/Square/5.billing_details_80_zoom.png)

### No credit card data is stored locally

#### Stripe

![Buy button](/demo/Stripe/buy.gif)

#### Square

![Square web sdk](/demo/Square/6.square_web_sdk.png)

### Strong customer authentication (3D-Secure, SCA) support

![3D-Secure](/demo/Square/7.3D_secure.png)

### File delivery after payment

#### Stripe

![File delivery after payment after stripe payment](/demo/Stripe/file-delivery.gif)

#### Square

![File delivery after payment after square payment](/demo/Square/8.File_download_delivery.png)

## Usage

### Requirements

1. [Stripe](https://stripe.com/) or [Square](https://squareup.com) account for payment gateway
2. [Google PaLM AI API](https://developers.generativeai.google/products/palm) key for autocomplete in editor.
3. [Cloudflare](https://www.cloudflare.com/) account for turnstile captcha.
4. [Mailchimp](https://mailchimp.com/) account for adding subscribers to the list.

Note: Open Payment Host can be tested without fulfilling above requirements, But payments and adding subscribers to the list wouldn't work.

### Docker

The latest image is available on DockerHub at [`abishekmuthian/open-payment-host:latest`](https://hub.docker.com/layers/abishekmuthian/open-payment-host/latest).

### Demo Setup

```bash
mkdir oph-demo && cd oph-demo
sh -c "$(curl -fsSL https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-demo/install-demo.sh)"
```

Visit `http://localhost:3000`.

Demo version prints more detailed level of errors if any, DO NOT use this demo setup in production.

#### Login

The default admin email id is `admin@openpaymenthost.com` and the password is `OpenPaymentHost`. You'll be asked to reset the password after login for security reasons. That password is hashed and stored.

You'll be logged out after changing password automatically to login with the new credentials.

Admin email id and default admin password can be changed in the config file (explained below) after the first run. Each time the password is reset in the config file, The password needs to be changed again once logged in for security.

### Production Setup

It's recommended to try the demo application first before using the production application. The production application requires a registered domain and special config variables for SSL as detailed in the configuration section.

```bash
mkdir oph-production && cd oph-production
sh -c "$(curl -fsSL https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-production/install-production.sh)"
```

After the container has started successfully, Stop the container, Set the required production configuration and re-run the container.

### Configuration

Config file `fragmenta.json` is located in the `secrets` folder. It is generated automatically during the first run, After editing the config file the application needs to be restarted for the new configuration to take effect.

`fragmenta.json` contains configuration for both development and production. The production configuration is loaded when the environment variable `FRAG_ENV=production` is set.

Note: Environment variable for production is set automatically when using the docker production setup.

User configurable values are included in the table below.

| Key                         | Description                                                                                 | Value                                                                               |
| --------------------------- | ------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| admin_email                 | Email id of the administrator.                                                              | Default: admin@openpaymenthost.com                                                  |
| admin_default_password      | Default password of the administrator, Would be forced to changed after login.              | Default: OpenPaymentHost                                                            |
| reset_admin                 | Reset the email and password of the admin during the next run.                              | yes (or) no                                                                         |
| domain                      | Website domain name for the application.                                                    | Dev: localhost, Prod: example                                                       |
| port                        | Port of the application.                                                                    | Dev: 3000, Prod: 443 (SSL)                                                          |
| root_url                    | FQDN for the application with protocol and port.                                            | Dev: http://localhost:3000, Prod: https://example.com                               |
| autocert_domains            | Comma separated domains for SSL certificates.                                               | Demo: NA, Prod: www.example.com, example.com                                        |
| autocert_email              | email id for SSL certificate related notifications.                                         | Demo: NA, Prod: admin@example.com                                                   |
| name                        | Name of the website.                                                                        | Default: Open Payment Host                                                          |
| meta_title                  | Title of the website.                                                                       | Default : Sell what you want without paying commissions                             |
| meta_desc                   | Description of the website.                                                                 | Default: Sell Subscriptions, Newsletters, Digital Files without paying commissions. |
| meta_keywords               | Keywords for the website.                                                                   | Default: payments,subscription,projects,products                                    |
| meta_image                  | URL for the featured image for the website.                                                 | Default: /assets/images/app/oph_featured_image.png                                  |
| meta_url                    | Meta URL for the page when its not generated automatically.                                 | Dev: http://localhost:3000, Prod: https://example.com                               |
| square                      | Enable the square payment gateway, When enabled all other square credentials are mandatory. | Dev/Prod : yes,no                                                                   |
| square_access_token         | Square access token for accessing your square account.                                      | Dev: Sandbox access token, Prod: Production access token                            |
| square_app_id               | Square app id to identify your application.                                                 | Dev: sandbox-..., Production: production app id                                     |
| square_location_id          | Square location id for the account                                                          | Dev: Sandbox location id from test account, Prod: location id from the main account |
| square_notification_url     | Square notification url for the webhook.                                                    | Dev:Sandbox webhook URL, Prod: Production webhook URL                               |
| square_signature_key        | Square signature for webhook authentication                                                 | Dev: Sandbox webhook signature, Prod: Production webhook signature                  |
| square_sandbox_source_id    | Square sandbox source id for credit card                                                    | Dev: cnon:card-nonce-ok, Prod:""                                                    |
| square_domain               | Square API domain                                                                           | Dev: https://connect.squareupsandbox.com/v2, Prod: https://connect.squareup.com/v2  |
| s3_access_key               | S3 compatible access key                                                                    | Dev: NA,Prod: NA                                                                    |
| s3_secret_key               | S3 compatible secret key                                                                    | Dev: NA, Prod: NA                                                                   |
| stripe                      | Enable the stripe payment gateway, When enabled all other stripe credentials are mandatory. | Dev/Prod : yes, no                                                                  |
| stripe_key                  | Stripe developer key.                                                                       | Dev: pk*test*..., Prod: pk*live*...\*\*\*\*                                         |
| stripe_secret               | Stripe developer secret key.                                                                | Dev: sk*test*..., Prod: sk*live*...                                                 |
| stripe_webhook_secret       | Stripe webhook signing secret                                                               | Dev: whsec_xxx, Prod: whsec_xxx                                                     |
| stripe_tax_rate_IN          | Stripe tax id for India.                                                                    | Dev: txr*..., Prod: txr*...                                                         |
| stripe_callback_domain      | Root URL for callback after Stripe event.                                                   | Dev: [Use tunnel like ngrok], Prod: [Use root_url]                                  |
| subscription_client_country | Test country for testing multi-country pricing.                                             | Dev: US, IN, FR etc. Prod: NA                                                       |
| mailchimp_token             | Mailchimp API Key.                                                                          | e.g. ...-us12                                                                       |
| turnstile_secret_key        | Cloudflare turnstile secret key for captcha.                                                | Dev: 1x00000000000000000000AA, Prod: 0x...                                          |
| turnstile_site_key          | Cloudflare turnstile key for captcha.                                                       | Dev: 1x0000000000000000000000000000000AA, Prod: 0x...                               |
| palm_key                    | Google AI PaLM API Key                                                                      | Dev: xxxxxxxxx, Prod: xxxxxxxxx                                                     |

### Stripe Webhook Setup

Webhook needs to be setup at [Stripe](https://stripe.com) Developer's section for receiving subscription details post payment.

Set the webhook to `root_url/payment/webhook` where the root_url is defined in the configuration above. To test the webhooks in the local environment, Use a tunnel like [ngrok](https://ngrok.com/).

Set the following events to send:

1. `checkout.session.completed`
2. `payment_method.attached`
3. `invoice.paid`
4. `invoice.payment_failed`
5. `customer.subscription.deleted`

### Square Webhook Setup

Webhook needs to be setup at [Square](https://developer.squareup.com) Developer's section for receiving subscription details post payment.

Set the webhook to `root_url/payment/square_webhook` where the root_url is defined in the configuration above. To test the webhooks in the local environment, Use a tunnel like [ngrok](https://ngrok.com/).

Set the following events to send:

1. `subscription.created `
2. `subscription.updated `

## Developer

### Build Open Payment Host

To build the application yourself,

```
$ git clone https://github.com/abishekmuthian/open-payment-host
$ cd open-payment-host
$ go build open-payment-host
```

There are `docker-compose` , `Dockerfile` files in the root of the project to build a docker image.

### Tailwind

Open Payment Host uses Tailwind and Daisy UI for its UI.

Compile Tailwind using the following command,

```
npx tailwindcss -i tailwind/tailwind.css src/app/assets/styles/app.css --watch
```

### License

Copyright (C) 2023 Abishek Muthian (Open Payment Host)

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License along with this program. If not, see https://www.gnu.org/licenses/.

### Licenses for open-source libraries used in this project

Fragmenta: https://github.com/fragmenta licensed under [The MIT License](https://github.com/andybrewer/mvp/blob/master/LICENSE).

tailwindcss: https://github.com/tailwindlabs/tailwindcss licensed under [The MIT License](https://github.com/tailwindlabs/tailwindcss/blob/master/LICENSE).

daisyui: https://github.com/saadeghi/daisyui licensed under [The MIT License](https://github.com/saadeghi/daisyui/blob/master/LICENSE).

trix: https://github.com/basecamp/trix licensed under [The MIT License](https://github.com/basecamp/trix/blob/main/LICENSE).
