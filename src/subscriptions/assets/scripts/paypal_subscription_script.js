document.addEventListener("DOMContentLoaded", async function () {
  if (!window.paypal || paymentScriptType() != "subscription") {
    console.log("Not loading Paypal subscription script on this page");
    return;
  }

  // Extracting the product ID from the current URL
  const urlParams = new URLSearchParams(window.location.search);
  const productId = decodeURIComponent(urlParams.get("product_id"));
  const customId = decodeURIComponent(urlParams.get("custom_id"));
  const redirectURI = decodeURIComponent(urlParams.get("redirect_uri"));

  const planId = planID();

  paypal
    .Buttons({
      createSubscription: function (data, actions) {
        return actions.subscription.create(
          createSubscriptionObject(planId, customId)
        );
      },

      onApprove: function (data, actions) {
        console.log(
          `You have successfully subscribed to  ${data.subscriptionID}`
        );

        let timerInterval;
        Swal.fire({
          title: "Payment verification in progress!",
          html: "It will take a minute and you will be redirected once complete.",
          timer: 25000,
          timerProgressBar: true,
          icon: "info",
          didOpen: () => {
            Swal.showLoading();
            const timer = Swal.getPopup().querySelector("b");
            timerInterval = setInterval(() => {
              timer.textContent = `${Swal.getTimerLeft()}`;
            }, 100);
          },
          willClose: () => {
            clearInterval(timerInterval);
          },
        }).then((result) => {
          /* Read more about handling dismissals below */
          if (result.dismiss === Swal.DismissReason.timer) {
            console.log("I was closed by the timer");
            window.location =
              window.location.origin +
              "/subscriptions/success?paypal_subscriptionid=" +
              `${data.subscriptionID}` +
              "&product_id=" +
              `${productId}` +
              "&redirect_uri=" +
              `${redirectURI}` +
              "&custom_id=" +
              `${customId}`;
          }
        });
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

function createSubscriptionObject(planId, customId) {
  const subscriptionObject = {};

  // ... other fields ...

  if (planId !== null && planId !== "") {
    subscriptionObject.plan_id = planId;
  }

  if (customId !== null && customId !== "") {
    subscriptionObject.custom_id = customId;
  }

  /*   subscriber: {
            email_address: "user@example.com",
            name: {
              given_name: "givenName",
            },
          }, */

  return subscriptionObject;
}
