document.addEventListener("DOMContentLoaded", async function () {
  if (!window.paypal || paymentScriptType() != "checkout") {
    console.log("Not loading Paypal checkout script on this page");
    return;
  }
  // Extracting the product ID from the current URL
  const urlParams = new URLSearchParams(window.location.search);
  const productId = decodeURIComponent(urlParams.get("product_id"));
  const customId = decodeURIComponent(urlParams.get("custom_id"));
  const redirectURI = decodeURIComponent(urlParams.get("redirect_uri"));
  paypal
    .Buttons({
      style: {
        shape: "rect",
        layout: "vertical",
        color: "gold",
        label: "paypal",
      },
      message: {
        amount: productAmount(),
      },

      async createOrder() {
        try {
          const response = await fetch("/subscriptions/paypal/orders", {
            method: "POST",
            headers: {
              "Content-Type": "application/x-www-form-urlencoded",
            },
            // use the "body" param to optionally pass additional order information
            // like product ids and quantities
            body:
              "authenticity_token=" +
              authenticityToken() +
              "&product_id=" +
              productID() +
              "&custom_id=" +
              customId,
          });

          const orderData = await response.json();

          if (orderData.id) {
            return orderData.id;
          }
          const errorMessage = "Paypal order id was not generated";
          throw new Error(errorMessage);
        } catch (error) {
          console.error(error);
          Swal.fire({
            title: "Error processing Paypal payment!",
            text: "Please contact support.",
            icon: "error",
            confirmButtonText: "Dismiss",
          });
        }
      },
      async onApprove(data, actions) {
        try {
          const response = await fetch(
            `/subscriptions/paypal/orders/${data.orderID}/capture`,
            {
              method: "POST",
              headers: {
                "Content-Type": "application/x-www-form-urlencoded",
              },
              body: "authenticity_token=" + authenticityToken(),
            }
          );

          const orderData = await response.json();

          const transaction =
            orderData?.purchase_units?.[0]?.payments?.captures?.[0];

          console.log(
            `Paypal Transaction ${transaction.status}: ${transaction.id}`
          );
          if (transaction.status === "COMPLETED") {
            window.location =
              window.location.origin +
              "/subscriptions/success?paypal_orderid=" +
              `${orderData.id}` +
              "&product_id=" +
              `${productId}` +
              "&redirect_uri=" +
              `${redirectURI}` +
              "&custom_id=" +
              `${customId}`;
          } else {
            const errorMessage = "Paypal order was not captured";
            throw new Error(errorMessage);
          }
          /*           // Three cases to handle:
          //   (1) Recoverable INSTRUMENT_DECLINED -> call actions.restart()
          //   (2) Other non-recoverable errors -> Show a failure message
          //   (3) Successful transaction -> Show confirmation or thank you message

          const errorDetail = orderData?.details?.[0];

          if (errorDetail?.issue === "INSTRUMENT_DECLINED") {
            // (1) Recoverable INSTRUMENT_DECLINED -> call actions.restart()
            // recoverable state, per
            // https://developer.paypal.com/docs/checkout/standard/customize/handle-funding-failures/
            return actions.restart();
          } else if (errorDetail) {
            // (2) Other non-recoverable errors -> Show a failure message
            throw new Error(
              `${errorDetail.description} (${orderData.debug_id})`
            );
          } else if (!orderData.purchase_units) {
            throw new Error(JSON.stringify(orderData));
          } else {
            // (3) Successful transaction -> Show confirmation or thank you message
            // Or go to another URL:  actions.redirect('thank_you.html');
            const transaction =
              orderData?.purchase_units?.[0]?.payments?.captures?.[0] ||
              orderData?.purchase_units?.[0]?.payments?.authorizations?.[0];
            resultMessage(
              `Transaction ${transaction.status}: ${transaction.id}<br>
          <br>See console for all available details`
            );
            console.log(
              "Capture result",
              orderData,
              JSON.stringify(orderData, null, 2)
            );
          }
            */
        } catch (error) {
          console.error(error);
          Swal.fire({
            title: "Error processing Paypal payment!",
            text: "Please contact support.",
            icon: "error",
            confirmButtonText: "Dismiss",
          });
        }
      },
    })
    .render("#paypal-button-container");
});
