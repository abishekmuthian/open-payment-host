document.addEventListener("DOMContentLoaded", async function () {
  if (!window.Razorpay || paymentScriptType() != "subscription") {
    console.log("Not loading Razorpay subscription script on this page");
    return;
  }

  // Extracting the product ID from the current URL
  const urlParams = new URLSearchParams(window.location.search);
  const productId = decodeURIComponent(urlParams.get("product_id"));
  const customId = decodeURIComponent(urlParams.get("custom_id"));
  const redirectURI = decodeURIComponent(urlParams.get("redirect_uri"));

  var options = {
    key: razorpayKeyID(), // Enter the Key ID generated from the Dashboard
    subscription_id: subscriptionID(),
    name: productTitle(),
    // description: "Test Transaction",
    // image: "https://example.com/your_logo",
    handler: function (response) {
      console.log(response.razorpay_payment_id);
      console.log(response.razorpay_subscription_id);
      console.log(response.razorpay_signature);
      window.location =
        window.location.origin +
        "/subscriptions/success?razorpay_payment_id=" +
        `${response.razorpay_payment_id}` +
        "&razorpay_subscription_id=" +
        `${response.razorpay_subscription_id}` +
        "&razorpay_signature=" +
        `${response.razorpay_signature}` +
        "&product_id=" +
        `${productId}` +
        "&redirect_uri=" +
        `${redirectURI}` +
        "&custom_id=" +
        `${customId}`;
    },
    prefill: {
      name: "",
      email: "",
    },
    notes: {
      custom_id: customId !== "null" ? customId : "",
      product_id: productId,
      name: "",
      email: "",
      address_state: "",
    },
    theme: {
      color: "#3399cc",
    },
  };
  var rzp1 = new Razorpay(options);
  rzp1.on("payment.failed", function (response) {
    console.log(response.error.code);
    console.log(response.error.description);
    console.log(response.error.source);
    console.log(response.error.step);
    console.log(response.error.reason);
    console.log(response.error.metadata.order_id);
    console.log(response.error.metadata.payment_id);
    Swal.fire({
      title: "Error processing Razorpay payment!",
      text: response.error.description,
      icon: "error",
      confirmButtonText: "Dismiss",
    });
  });
  document.getElementById("rzp-button1").onclick = function (e) {
    // if .razorpay-input-name input has no value
    if (
      !document.querySelector(".razorpay-input-name").value ||
      !document.querySelector(".razorpay-input-name").checkValidity()
    ) {
      Swal.fire({
        text: "Please enter your name",
        icon: "error",
        theme: "auto",
      });
    }
    // if .razorpay-input-email input has no value
    else if (
      !document.querySelector(".razorpay-input-email").value ||
      !document.querySelector(".razorpay-input-email").checkValidity()
    ) {
      Swal.fire({
        text: "Please enter a valid email",
        icon: "error",
        theme: "auto",
      });
    }
    // if .razorpay-input-state select has no value
    else if (
      document.querySelector(".razorpay-input-state").value == "Select State"
    ) {
      Swal.fire({
        text: "Please select your state",
        icon: "error",
        theme: "auto",
      });
    } else {
      options.prefill.name = document.querySelector(
        ".razorpay-input-name"
      ).value;
      options.notes.name = document.querySelector(".razorpay-input-name").value;
      options.prefill.email = document.querySelector(
        ".razorpay-input-email"
      ).value;
      options.notes.email = document.querySelector(
        ".razorpay-input-email"
      ).value;
      options.notes.address_state = document.querySelector(
        ".razorpay-input-state"
      ).value;
      rzp1.open();
      e.preventDefault();
    }
  };
});
