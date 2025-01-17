let App = {};

let network = "testnet";
// ex: 'https://digitalcash.dev' or '.'
let baseUrl = ".";

// replaced by public-config.json
let allowedEndpoints = [
  "/api/goboilerplate/config",
  "/api/goboilerplate/config/foo",
  "/api/goboilerplate/config/bar",
];

// ajquery - still great after all these years!
function $(sel, el) {
  return (el || document).querySelector(sel);
}

function $$(sel, el) {
  return Array.from((el || document).querySelectorAll(sel));
}

async function request(endpoint, payload) {
  let method = "GET";
  let headers = {};
  if (payload) {
    method = "POST";
    Object.assign(headers, {
      "Content-Type": "application/json",
    });
  }
  let resp = await fetch(`${baseUrl}${endpoint}`, {
    method: method,
    headers: headers,
    body: payload,
  });

  let data = await resp.json();
  if (data.error) {
    let err = new Error(data.error.message);
    Object.assign(err, data.error);
    throw err;
  }

  let result = data.result || data;
  return result;
}

function parseHashQuery(locationHash) {
  let fragment = locationHash.slice(2); // drop leading '#?'
  if (!fragment) {
    return null;
  }

  let queryIter = new URLSearchParams(fragment);
  let query = Object.fromEntries(queryIter);
  let endpoint = query.endpoint;
  let bodyJson = queryIter.get("body");
  let body;
  if (bodyJson) {
    body = JSON.parse(bodyJson);
  }

  return {
    endpoint: endpoint,
    body: body,
    submit: "submit" in query,
  };
}

App.$updatePreview = async function (event) {
  let defaults = { port: "9998" };
  if (network === "testnet") {
    defaults = {
      port: "19998",
    };
  }

  let endpoint = $("input[name=goboilerplate-endpoint]").value || "";
  let body = ""; // $("input[name=goboilerplate-payload]").value;
  let opts = { pathname: endpoint, body: body };
  let isKnown = allowedEndpoints.includes(endpoint);

  let shareHash = "";
  let shareUrl = "";
  let bodyJson; // $("input[name=goboilerplate-payload]").value || '{}';
  if (isKnown) {
    if (bodyJson) {
      shareHash = `#?endpoint=${endpoint}&body=${bodyJson}&submit`;
    } else {
      shareHash = `#?endpoint=${endpoint}&submit`;
    }
  }

  if (isKnown || !endpoint) {
    shareUrl = `${location.protocol}//${location.host}/${shareHash}`;
    $("[data-id=share]").href = shareUrl;
    $("[data-id=share]").innerText = shareUrl;
  } else {
    return;
  }

  let previewType = document.querySelector("[name=http-request]:checked").value;
  let code = "";
  if (previewType === "curl") {
    code = renderCurl(opts);
  } else if (previewType === "fetch") {
    code = renderFetch(opts);
  } else {
    throw new Error(`must select either 'curl' or 'fetch' preview style`);
  }
  code = code.replace(/\n/, "&#10;");
  code = code.replace(/\t/, "&#09;");
  document.querySelector("[data-id=http-request-preview]").innerHTML = code;

  location.hash = shareHash;

  return true;
};

App.$submitForm = async function (event) {
  event.preventDefault();

  let endpoint = $("input[name=goboilerplate-endpoint]").value;
  let isKnown = allowedEndpoints.includes(endpoint);
  if (!isKnown) {
    window.alert(`unknown endpoint '${endpoint}'`);
    return;
  }

  let payload;
  let result = await request(endpoint, payload).catch(function (err) {
    let data = {
      code: err.code,
      message: err.message,
    };
    return data;
  });
  let json = JSON.stringify(result, null, 2);

  $("[data-id=output]").textContent = json;
};

function renderCurl(opts) {
  let body = opts.body || "";
  body = body.replace(/^/gm, "    ");
  body = body.trim();
  let dataBinary = "";
  if (body) {
    dataBinary = `\\\n    --data-binary '${body}'\n`;
  }
  let code = `

curl --fail-with-body https://${window.location.host}${opts.pathname} \\
    --user "$user:$pass" \\
    -H "Content-Type: application/json" ${dataBinary}
        `;
  code = code.trim();
  return code;
}

function renderFetch(opts) {
  let body = opts.body.trim();
  body = body.replace(/^/gm, "    ");
  body = body.trim();
  let code = `

let method = 'GET';
let baseUrl = "https://${window.location.host}/${opts.pathname}";
let basicAuth = btoa(\`user:pass\`);
let body = JSON.stringify(${body});
if (body) {
    method = 'POST';
}
let resp = await fetch(baseUrl, {
    method: method,
    headers: {
        "Authorization": \`Basic \${basicAuth}\`,
        "Content-Type": "application/json",
    },
    body: body,
});

let data = await resp.json();
if (data.error) {
    let err = new Error(data.error.message);
    Object.assign(err, data.error);
    throw err;
}
return data.result;

        `;
  code = code.trim();
  return code;
}

async function main() {
  let resp = await fetch(`./public-config.json`);
  if (!resp.ok) {
    let msg = `failed to fetch public config: ${resp.status} ${resp.statusText}`;
    window.alert(msg);
    return;
  }
  let data = await resp.json();

  {
    allowedEndpoints = [];
    for (let endpoint of data.endpoints) {
      // ... include, exclude, traverse here
      allowedEndpoints.push(endpoint);
    }

    let optionList = [];
    for (let opt of allowedEndpoints) {
      optionList.push(`<option value="${opt}">${opt}</option>`);
    }
    let options = optionList.join("\n");

    let $goboilerplateEndpoints = document.querySelector("#goboilerplate-endpoints");
    $goboilerplateEndpoints.innerText = "";
    $goboilerplateEndpoints.insertAdjacentHTML("afterbegin", options);
  }

  let opts = parseHashQuery(location.hash);
  if (opts?.endpoint) {
    $("input[name=goboilerplate-endpoint]").value = opts.endpoint;
  }
  if (opts?.body) {
    $("[data-id=args]").value = JSON.stringify(opts.body);
  }

  document.body.removeAttribute("hidden");

  await App.$updatePreview();

  if (opts?.submit && opts?.endpoint) {
    let isKnown = allowedEndpoints.includes(opts.endpoint);
    if (!isKnown) {
      return;
    }

    let event = new Event("submit", {
      bubbles: true,
      cancelable: true,
    });
    $("form#goboilerplate-form").dispatchEvent(event);
  }
}

main().catch(handleError);

function createDebounced(fn, ms) {
  let t = { timeout: 0 };

  async function debouncer(...args) {
    if (t.timeout) {
      clearTimeout(t.timeout);
    }
    await sleep(ms, t);
    let result = await fn.apply(this, args);
    return result;
  }

  return debouncer;
}

async function sleep(ms, t) {
  return new Promise(function (resolve) {
    t.timeout = setTimeout(resolve, ms);
  });
}

/** @param {Error} err */
function handleError(err) {
  console.error("main() caught uncaught error:");
  console.error(err);
  window.alert(
    `Error:\none of our developers let a bug slip through the cracks:\n\n${err.message}`,
  );
}

window.onerror = function (message, url, lineNumber, columnNumber, err) {
  if (!err) {
    err = new Error(
      `"somebody pulled a 'throw undefined', somewhere:\n message:'${message}' \nurl:'${url}' \nlineNumber:'${lineNumber}' \ncolumnNumber:'${columnNumber}'`,
    );
  }
  handleError(err);
};

window.onunhandledrejection = async function (event) {
  let err = event.reason;
  if (!err) {
    let msg = `developer error (not your fault): error is missing error object`;
    err = new Error(msg);
  }
  handleError(err);
};

export default App;
