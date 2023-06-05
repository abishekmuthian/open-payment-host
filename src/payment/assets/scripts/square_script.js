/* This is the part where you call the above defined function and "call back" your code which gets executed after the script has loaded */

document.addEventListener("DOMContentLoaded", async function () {
  if (!window.Square) {
    console.log("Not loading Square script on this page");
    return;
  }

  const appId = appID();
  const locationId = locationID();

  let payments;
  try {
    payments = window.Square.payments(appId, locationId);
  } catch {
    const statusContainer = document.getElementById("payment-status-container");
    statusContainer.className = "missing-credentials";
    statusContainer.style.visibility = "visible";
    return;
  }

  let card;
  try {
    card = await initializeCard(payments);
  } catch (e) {
    console.error("Initializing Card failed", e);
    return;
  }

  async function handlePaymentMethodSubmission(event, card) {
    event.preventDefault();

    try {
      // disable the submit button as we await tokenization and make a payment request.
      cardButton.disabled = true;
      const token = await tokenize(card);
      const verificationToken = await verifyBuyer(payments, token);
      const paymentResults = await createPayment(token, verificationToken);
      displayPaymentResults("SUCCESS");

      console.debug("Payment Success", paymentResults);
    } catch (e) {
      cardButton.disabled = false;
      displayPaymentResults("FAILURE");
      console.error(e.message);
    }
  }

  const cardButton = document.getElementById("card-button");
  cardButton.addEventListener("click", async function (event) {
    await handlePaymentMethodSubmission(event, card);
  });
});

// status is either SUCCESS or FAILURE;
function displayPaymentResults(status) {
  const statusContainer = document.getElementById("payment-status-container");
  if (status === "SUCCESS") {
    statusContainer.classList.remove("is-failure");
    statusContainer.classList.add("is-success");
  } else {
    statusContainer.classList.remove("is-success");
    statusContainer.classList.add("is-failure");
  }

  statusContainer.style.visibility = "visible";
}

// Required in SCA Mandated Regions: Learn more at https://developer.squareup.com/docs/sca-overview
async function verifyBuyer(payments, token) {
  const queryString = window.location.search;
  console.log(queryString);

  const urlParams = new URLSearchParams(queryString);

  const addressLines = [];
  addressLines.push(urlParams.get("addressLine1"));
  addressLines.push(urlParams.get("addressLine2"));

  const intent = urlParams.get("intent");

  const verificationDetails = {
    amount: urlParams.get("amount"),
    billingContact: {
      addressLines: addressLines,
      givenName: urlParams.get("givenName"),
      email: urlParams.get("email"),
      country: urlParams.get("country"),
      city: urlParams.get("city"),
      state: urlParams.get("state"),
      postalCode: urlParams.get("postalcode"),
    },
    currencyCode: urlParams.get("currency"),
    intent: intent,
  };

  const verificationResults = await payments.verifyBuyer(
    token,
    verificationDetails
  );
  return verificationResults.token;
}

async function tokenize(paymentMethod) {
  const tokenResult = await paymentMethod.tokenize();
  if (tokenResult.status === "OK") {
    return tokenResult.token;
  } else {
    let errorMessage = `Tokenization failed with status: ${tokenResult.status}`;
    if (tokenResult.errors) {
      errorMessage += ` and errors: ${JSON.stringify(tokenResult.errors)}`;
    }

    throw new Error(errorMessage);
  }
}

async function createPayment(token, verificationToken) {
  // Get authenticity token from head of page
  var authenticitytoken = authenticityToken();

  /*   const body = JSON.stringify({
    locationId,
    sourceId: token,
    verificationToken,
  }); */

  const queryString = window.location.search;
  console.log(queryString);

  const urlParams = new URLSearchParams(queryString);

  var url = "/subscriptions/square";

  if (urlParams.get("type") == "subscription") {
    url = "/subscriptions/subscribe";
  }

  // Removing the cnon: from the payment token to avoid issues with the routing
  var data =
    "authenticity_token=" +
    authenticitytoken +
    "&" +
    "paymentToken=" +
    token +
    "&" +
    "verificationToken=" +
    verificationToken +
    "&" +
    urlParams;

  DOM.Post(url, data, function (request) {
    // Use the response url to redirect
    window.location = request.responseURL;

    // Respond to error
    console.log("error", request);
    try {
      throw new Error(request);
    } catch {
      return;
    }
  });

  /*   const paymentResponse = await fetch("/payment", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body,
  });

  if (paymentResponse.ok) {
    return paymentResponse.json();
  }

  const errorBody = await paymentResponse.text();
  throw new Error(errorBody); */
}

async function initializeCard(payments) {
  const card = await payments.card();
  await card.attach("#card-container");

  return card;
}
