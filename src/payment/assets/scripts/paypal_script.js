document.addEventListener("DOMContentLoaded", async function () {
  if (!window.paypal) {
    console.log("Not loading Paypal script on this page");
    return;
  }

  const planId = planID();

  // paypal
  //   .Buttons({
  //     createSubscription: function (data, actions) {
  //       return actions.subscription.create({
  //         plan_id: planId, // Creates the subscription
  //         custom_id: "123customID",
  //         subscriber: {
  //           email_address: "user@example.com",
  //           name: {
  //             given_name: "givenName",
  //           },
  //         },
  //       });
  //     },

  //     onApprove: function (data, actions) {
  //       alert(`You have successfully subscribed to  ${data.subscriptionID}`); // Optional message given to subscriber
  //     },
  //   })
  //   .render("#paypal-button-container"); // Renders the PayPal button

  paypal
    .HostedButtons({
      hostedButtonId: "C843U8ACWFMJE",
    })
    .render("#paypal-hosted-button-container");
});
