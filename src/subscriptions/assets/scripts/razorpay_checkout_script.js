document.addEventListener("DOMContentLoaded", async function () {
  if (!window.Razorpay || paymentScriptType() != "checkout") {
    console.log("Not loading Razorpay checkout script on this page");
    return;
  }

  // Extracting the product ID from the current URL
  const urlParams = new URLSearchParams(window.location.search);
  const productId = urlParams.get("product_id") || "";
  const customId = urlParams.get("custom_id") || "";
  const redirectURI = urlParams.get("redirect_uri") || "";

  document.getElementById("rzp-button1").onclick = function (e) {
    // Check if phone field exists (for Indian users only)
    const phoneField = document.querySelector(".razorpay-input-phone");
    const stateField = document.querySelector(".razorpay-input-state");

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
    // if .razorpay-input-phone input exists and has no value (Indian users only)
    else if (phoneField && (!phoneField.value || !phoneField.checkValidity())) {
      Swal.fire({
        text: "Please enter a valid phone number",
        icon: "error",
        theme: "auto",
      });
    }

    // if .razorpay-input-address input has no value
    else if (
      !document.querySelector(".razorpay-input-address").value ||
      !document.querySelector(".razorpay-input-address").checkValidity()
    ) {
      Swal.fire({
        text: "Please enter a valid address",
        icon: "error",
        theme: "auto",
      });
    }

    // if .razorpay-input-city input has no value
    else if (
      !document.querySelector(".razorpay-input-city").value ||
      !document.querySelector(".razorpay-input-city").checkValidity()
    ) {
      Swal.fire({
        text: "Please enter a valid city",
        icon: "error",
        theme: "auto",
      });
    }

    // if .razorpay-input-state select has no value (for Indian users with dropdown)
    // or if it's an input field (for non-Indian users), check if it has a value
    else if (
      (stateField.tagName === "SELECT" &&
        stateField.value === "Select State") ||
      (stateField.tagName === "INPUT" &&
        (!stateField.value || !stateField.checkValidity()))
    ) {
      Swal.fire({
        text:
          stateField.tagName === "SELECT"
            ? "Please select your state"
            : "Please enter your state",
        icon: "error",
        theme: "auto",
      });
    }

    // if .razorpay-input-pincode input has no value
    else if (
      !document.querySelector(".razorpay-input-pincode").value ||
      !document.querySelector(".razorpay-input-pincode").checkValidity()
    ) {
      Swal.fire({
        text: "Please enter a valid pincode",
        icon: "error",
        theme: "auto",
      });
    } else {
      var options = {
        key: razorpayKeyID(), // Enter the Key ID generated from the Dashboard
        amount: productAmount(), // Amount is in currency subunits. Default currency is INR. Hence, 50000 refers to 50000 paise
        currency: productCurrency(),
        name: productTitle(),
        // description: "Test Transaction",
        // image: "https://example.com/your_logo",
        order_id: orderID(), //This is a sample Order ID. Pass the `id` obtained in the response of Step 1
        handler: function (response) {
          console.log(response.razorpay_payment_id);
          console.log(response.razorpay_order_id);
          console.log(response.razorpay_signature);

          // Build success URL with proper parameter handling
          let successURL =
            window.location.origin +
            "/subscriptions/success?razorpay_payment_id=" +
            encodeURIComponent(response.razorpay_payment_id) +
            "&razorpay_order_id=" +
            encodeURIComponent(response.razorpay_order_id) +
            "&razorpay_signature=" +
            encodeURIComponent(response.razorpay_signature) +
            "&product_id=" +
            encodeURIComponent(productId);

          // Only add redirect_uri and custom_id if they are not empty
          if (redirectURI && redirectURI !== "") {
            successURL += "&redirect_uri=" + encodeURIComponent(redirectURI);
          }
          if (customId && customId !== "") {
            successURL += "&custom_id=" + encodeURIComponent(customId);
          }

          window.location = successURL;
        },
        prefill: {
          name: document.querySelector(".razorpay-input-name").value,
          email: document.querySelector(".razorpay-input-email").value,
          contact: phoneField ? phoneField.value : "",
        },
        notes: {
          custom_id: customId || "",
          product_id: productId,
          name: document.querySelector(".razorpay-input-name").value,
          email: document.querySelector(".razorpay-input-email").value,
          phone: phoneField ? phoneField.value : "",
          address: document.querySelector(".razorpay-input-address").value,
          address_city: document.querySelector(".razorpay-input-city").value,
          address_state: stateField.value,
          address_pincode: document.querySelector(".razorpay-input-pincode")
            .value,
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

      rzp1.open();
      e.preventDefault();
    }
  };
});
