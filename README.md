
# Open Payment Host
Sell Subscriptions, Newsletters, Digital Files without paying commissions.

![Open Payment Host home page](/demo/home.png)

## What
Open Payment Host is an easy to run self-hosted, minimalist Payments Host through which we can easily sell our digital items without paying double commissions while having total control over our sales and data.

## Why
Selling digital items on web as an indie requires using platforms where we have to pay double commissions(to the platform and the payment gateway) and our content is forever locked within those platforms.

## How
Open Payment Host is a minimalist yet highly performant Go web application with innovative features which helps indies self-host and sell digital items with little effort.

## Video Demo
[![Video Demo](demo/thumbnail-site.png)](https://youtu.be/cRK97mB1GUI)

Clicking the above image would open the video in YouTube.

## Features
* Customers can buy without logging in, Increases conversion.
* Stripe support, Just add the price id for the product and rest is done automatically.
* Multi-country pricing, Price changes automatically according to the user's location resulting in better conversion.
* Mailchimp support, Customers are automatically added to a mailchimp list; Useful for sending newsletters.
* WYSIWYG editor to create beautiful product pages.
* Automatic SSL and other security features for production.

and many more.

## Screenshots

### WYSIWYG editor in action
![WYSIWYG editor in action](/demo/editor.gif)

### Post display
![Post Display](/demo/post-display.gif)

### Buy without login
![Buy](/demo/buy.gif)

### File delivery after payment
![File delivery after payment](/demo/file-delivery.gif)

## Usage

### Requirements
1. [Stripe](https://stripe.com/) account for payment gateway.
2. [Cloudflare](https://www.cloudflare.com/) account for turnstile captcha.
3. [Mailchimp](https://mailchimp.com/) account for adding subscribers to the list.

Note: Open Payment Host can be tested without fulfilling above requirements, But payments and adding subscribers to the list wouldn't work.

### Docker
The latest image is available on DockerHub at [`abishekmuthian/open-payment-host:latest`](https://hub.docker.com/layers/abishekmuthian/open-payment-host/latest).

The latest slimmed image is available on DockerHub at [`abishekmuthian/open-payment-host:latest-slim`](https://hub.docker.com/layers/abishekmuthian/open-payment-host/latest-slim)

### Demo Setup

```bash
mkdir oph-demo && cd oph-demo
sh -c "$(curl -fsSL https://raw.githubusercontent.com/abishekmuthian/open-payment-host/main/samples/oph-demo/install-demo.sh)"
```

Visit `http://localhost:3000`.

Demo version prints more detailed level of errors if any, DO NOT use this demo setup in production.

**Login**:

The default admin email id is `test@test.com` and the password is `OpenPaymentHost`. You'll be asked to reset the password after login for security reasons. That password is hashed and stored.

You'll be logged out after changing password automatically to login with the new credentials.

Admin email id and Password can be changed in the config file (explained below) after the first run.

### Production Setup

It's recommended to try the demo application first before using the production application. The production application requires special config variables detailed in the configuration section.

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

| Key | Description | Value |
| ----------- | ----------- | --------|
| admin_email | Email id of the administrator. | Default: test@test.com |
| admin_default_password | Default password of the administrator, Would be forced to changed after login. | Default: OpenPaymentHost |
| reset_admin | Reset the email and password of the admin during the next run. | true (or) false
| domain | Website domain for the application. | Dev: localhost, Prod: example
| port | Port of the application. | Dev: 3000, Prod: 443 (SSL)
| root_url | FQDN for the application with protocol and port. | Dev: http://localhost:3000, Prod: https://example.com
| autocert_domains | Comma separated domains for SSL certificates. | Demo: NA, Prod: www.example.com, example.com
| autocert_email | email id for SSL certificate related notifications. | Demo:NA, Prod: admin@example.com
| name | Name of the website. | Default: Open Payment Host
| meta_title | Title of the website. | Default : Sell what you want without paying commissions
| meta_desc | Description of the website. | Default: Sell Subscriptions, Newsletters, Digital Files without paying commissions.
| meta_keywords | Keywords for the website. | Default: payments,subscription,projects,products
| meta_image | URL for the featured image for the website. | Default: /assets/images/app/oph_featured_image.png
| meta_url | Meta URL for the page when its not generated automatically. | Dev: http://localhost:3000, Prod: https://example.com
| stripe_key | Stripe developer key. | Dev: pk_test_..., Prod: pk_live_...
| stripe_secret | Stripe developer secret key. | Dev: sk_test_..., Prod: sk_live_...
| stripe_tax_rate_IN | Stripe tax id for India. | Dev: txr_..., Prod: txr_...
| stripe_callback_domain | Root URL for callback after Stripe event. | Dev: [Use tunnel like ngrok], Prod: [Use root_url]
| subscription_client_country | Test country for testing multi-country pricing. | Dev: US, IN, FR etc. Prod: NA
| mailchimp_token | Mailchimp API Key. |  e.g. ...-us12
| turnstile_secret_key | Cloudflare turnstile secret key for captcha. | Dev: 1x00000000000000000000AA, Prod: 0x...
| turnstile_site_key | Cloudflare turnstile key for captcha. | Dev: 1x0000000000000000000000000000000AA, Prod: 0x...


## Developer


### Build Open Payment Host

To build the application yourself,

```
$ git clone https://github.com/abishekmuthian/open-payment-host
$ cd open-payment-host
$ go build open-payment-host
```

There are `docker-compose` , `Dockerfile` files in the root of the project to build a docker image. The docker image can be secured and compressed using slim.ai as detailed in the next section(and shown in the Demo video at the start).

### Tailwind
Open Payment Host used Tailwind and Daisy UI for its UI. 

Compile Tailwind using the following command,

```
npx tailwindcss -i tailwind/tailwind.css src/app/assets/styles/app.css --watch
```

## Slim AI
Open Payment Host uses [slim.ai](https://slim.ai) to secure, harden and reduce the size of the container.

You can use the following tools to explore Open Payment Host container [on slim.ai](https://portal.slim.dev/home/profile/dockerhub%3A%2F%2Fdockerhub.public%2Fabishekmuthian%2Fopen-payment-host%3Alatest).

### Overview

slim.ai provides overview of the image to spot issues like **root user** and the size of the container image.

![Slim Ai Overview](/demo/slimai_overview.png)

### Vulnerability Scanner

slim.ai has multiple vulnerability scanners which scan for known vulnerabilities in the code of our container image.

![Slim Ai Vulnerability Scanner](/demo/slimai_vulnerability_scanner.png)

### Harden

slim.ai's harden feature reduces the overall size of the image by removing unwanted files and replacing bloated system files with hardened equivalent.

Open Payment Host image was compressed 91% using slim.ai tools.

![Slim Ai Harden](/demo/slimai_harden_compress.png)

### License
The MIT License (MIT)

Copyright (c) 2022 ABISHEK MUTHIAN (www.openpaymenthost.com)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

### Licenses for open-source libraries used in this project
Fragmenta: https://github.com/fragmenta licensed under [The MIT License](https://github.com/andybrewer/mvp/blob/master/LICENSE).

tailwindcss: https://github.com/tailwindlabs/tailwindcss licensed under [The MIT License](https://github.com/tailwindlabs/tailwindcss/blob/master/LICENSE).

daisyui: https://github.com/saadeghi/daisyui licensed under [The MIT License](https://github.com/saadeghi/daisyui/blob/master/LICENSE).







