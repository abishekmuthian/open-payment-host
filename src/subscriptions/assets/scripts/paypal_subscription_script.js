document.addEventListener("DOMContentLoaded", async function () {
  if (!window.paypal || paymentScriptType() != "subscription") {
    console.log("Not loading Paypal subscription script on this page");
    return;
  }

  // Extracting the product ID from the current URL
  const urlParams = new URLSearchParams(window.location.search);
  const productId = decodeURIComponent(urlParams.get("product_id"));
  const customId = (urlParams.get("custom_id") || "");
  const redirectURI = (urlParams.get("redirect_uri") || "");

  paypal
    .Buttons({
      createSubscription: function (data, actions) {
        // The subscription is created server-side so the plan is derived
        // from the product configuration, never from the browser
        return fetch("/subscriptions/paypal/subscriptions", {
          method: "POST",
          headers: {
            "Content-Type": "application/x-www-form-urlencoded",
          },
          body:
            "authenticity_token=" +
            authenticityToken() +
            "&product_id=" +
            productID() +
            "&custom_id=" +
            encodeURIComponent(customId) +
            "&redirect_uri=" +
            encodeURIComponent(redirectURI),
        })
          .then((response) => {
            if (!response.ok) {
              throw new Error("Subscription could not be created");
            }
            return response.json();
          })
          .then((subscriptionData) => {
            if (!subscriptionData.id) {
              throw new Error("Subscription could not be created");
            }
            return subscriptionData.id;
          });
      },

      onApprove: function (data, actions) {
        console.log(
          `You have successfully subscribed to  ${data.subscriptionID}`
        );

        window.location = "/subscriptions/success?" + new URLSearchParams({paypal_subscriptionid: data.subscriptionID});
      },
      onCancel(data) {
        // Show a cancel page, or return to cart
        window.location = window.location.origin + "/subscriptions/failure";
      },
      onError(err) {
        // For example, redirect to a specific error page
        console.error(err);
        Swal.fire({
          title: "Error processing Paypal payment!",
          text: "Please contact support.",
          icon: "error",
          confirmButtonText: "Dismiss",
        });
      },
    })
    .render("#paypal-button-container"); // Renders the PayPal button
});
