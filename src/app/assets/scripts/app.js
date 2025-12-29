DOM.Ready(function () {
  // Perform AJAX post on click on method=post|delete anchors
  ActivateMethodLinks();

  // Show/Hide elements with selector in attribute data-show
  ActivateShowlinks();

  // Submit forms of class .filter-form when filter fields change
  ActivateFilterFields();

  // Insert CSRF tokens into forms
  ActivateForms();

  // Insert CSRF tokens into HTMX
  ActivateHTMX();

  // Manage Billing
  ActivateManageBilling();

  // Clear Session Storage
  ClearSessionStorage();

  // Manage the burger menu
  ActivateBurgerMenu();
});

// Handles the burger menu for the mobile screens
function ActivateBurgerMenu() {
  DOM.On("#burger", "click", function (e) {
    var burger = this;
    const menu = DOM.First("#menu");

    if (DOM.HasClass(menu, "hidden")) {
      DOM.RemoveClass(menu, "hidden");
    } else {
      DOM.AddClass(menu, "hidden");
    }
  });
}

// Perform AJAX post on click on method=post|delete anchors
function ActivateMethodLinks() {
  DOM.On('a[method="post"], a[method="delete"]', "click", function (e) {
    var link = this;

    // Confirm action before delete
    if (link.getAttribute("method") == "delete") {
      if (
        !confirm(
          "Are you sure you want to delete this item, this action cannot be undone?",
        )
      ) {
        e.preventDefault();
        return false;
      }
    }

    // Ignore disabled links
    if (DOM.HasClass(link, "disabled")) {
      e.preventDefault();
      return false;
    }

    // Get authenticity token from head of page
    var token = authenticityToken();

    // Perform a post to the specified url (href of link)
    var url = link.getAttribute("href");
    var data = "authenticity_token=" + token;

    DOM.Post(
      url,
      data,
      function (request) {
        if (DOM.HasClass(link, "vote")) {
          // If a vote, up the points on the page
          var pointsContainer = link.parentNode.querySelectorAll(".points")[0];
          if (pointsContainer !== undefined) {
            console.log(pointsContainer);
            var points = parseInt(pointsContainer.innerText);
            var newPoints = points + 1;
            if (link.getAttribute("href").indexOf("upvote") == -1) {
              newPoints = points - 1;
            }
            pointsContainer.innerText = newPoints;
          }
        } else if (DOM.HasClass(link, "insights")) {
          // Do nothing
        } else {
          // Use the response url to redirect
          window.location = request.responseURL;
        }
      },
      function (request) {
        // Respond to error
        console.log("error", request);
      },
    );

    e.preventDefault();
    return false;
  });

  DOM.On('a[method="back"]', "click", function (e) {
    history.back(); // go back one step in history
    e.preventDefault();
    return false;
  });
}

// Insert an input into every form with js to include the csrf token.
// this saves us having to insert tokens into every form.
function ActivateForms() {
  // Get authenticity token from head of page
  var token = authenticityToken();

  DOM.Each("form", function (f) {
    // Create an input element
    var csrf = document.createElement("input");
    csrf.setAttribute("name", "authenticity_token");
    csrf.setAttribute("value", token);
    csrf.setAttribute("type", "hidden");

    //Append the input
    f.appendChild(csrf);
  });
}

// Submit forms of class .filter-form when filter fields change
function ActivateFilterFields() {
  DOM.On(
    ".filter-form .field select, .filter-form .field input",
    "change",
    function (e) {
      this.form.submit();
    },
  );
}

// Show/Hide elements with selector in attribute href - do this with a hidden class name
function ActivateShowlinks() {
  DOM.On(".show", "click", function (e) {
    e.preventDefault();
    var selector = this.getAttribute("data-show");
    if (selector == "") {
      selector = this.getAttribute("href");
    }

    DOM.Each(selector, function (el, i) {
      if (DOM.HasClass(el, "hidden")) {
        DOM.RemoveClass(el, "hidden");
      } else {
        DOM.AddClass(el, "hidden");
      }
    });

    return false;
  });
}

function ActivateManageBilling() {
  DOM.On(".manage-billing", "click", function (e) {
    console.log("Inside stripe success");
    e.preventDefault();
    fetch("/subscriptions/manage-billing", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        authenticityToken: authenticityToken(),
      }),
    })
      .then((response) => response.json())
      .then((data) => {
        window.location.href = data.url;
      })
      .catch((error) => {
        console.error("Error:", error);
      });
  });
}

// TODO: Rewrite the functions below into a single function which returns data from meta tags

// Collect the authenticity token from meta tags in header
function authenticityToken() {
  var meta = DOM.First("meta[name='authenticity_token']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the product ID from meta tags in header
function productID() {
  var meta = DOM.First("meta[name='product_ID']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the product title from meta tags in header
function productTitle() {
  var meta = DOM.First("meta[name='product_title']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the product quantity from meta tags in header
function productQuantity() {
  var meta = DOM.First("meta[name='product_quantity']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the product amount from meta tags in header
function productAmount() {
  var meta = DOM.First("meta[name='product_amount']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the product currency from the meta tags in header
function productCurrency() {
  var meta = DOM.First("meta[name='product_currency']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the app ID from meta tags in header
function appID() {
  var meta = DOM.First("meta[name='app_ID']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the location ID from meta tags in header
function locationID() {
  var meta = DOM.First("meta[name='location_ID']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the plan ID from the meta tags in header
function planID() {
  var meta = DOM.First("meta[name='plan_ID']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the subscription ID from the meta tags in header
function subscriptionID() {
  var meta = DOM.First("meta[name='product_subscription_ID']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the Razorpay Key ID from the meta tags in header
function razorpayKeyID() {
  var meta = DOM.First("meta[name='razorpay_key_id']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Check if is Paypal script is for Subscription or Checkout
function paymentScriptType() {
  var meta = DOM.First("meta[name='payment_script_type']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Collect the product order ID from the meta tags in header
function orderID() {
  var meta = DOM.First("meta[name='product_order_ID']");
  if (meta === undefined) {
    return "";
  }
  return meta.getAttribute("content");
}

// Clear Session Storage
function ClearSessionStorage() {
  if (
    window.location.href.substring(
      window.location.href.lastIndexOf("/") + 1,
    ) !== "create"
  ) {
    sessionStorage.setItem("description", "");
  }
}

// Insert authentication token into every HTMX request
function ActivateHTMX() {
  if (!window.htmx) {
    console.log("Not loading HTMX script on this page");
    return;
  }

  // Get authenticity token from head of page
  var token = authenticityToken();

  // add the auth token to the request as a header (for compatibility)
  // and also add to request parameters (required by backend)
  htmx.on("htmx:configRequest", (e) => {
    e.detail.headers["authenticity_token"] = token;
    e.detail.parameters["authenticity_token"] = token;
  });
}
