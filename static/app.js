let App = {};

const DEBUG_TICKER = "debug:ticker";

let network = "testnet";
// ex: 'https://digitalcash.dev' or '.'
let baseUrl = ".";
let eventSourceId = crypto.randomUUID();
let eventSource;
let isUnloading = false;

// replaced by public-config.json
let allowedTopics = [
  "rawtx",
  "rawblock",
  "rawgovernancevote",
  "rawgovernanceobject",
];
let defaultTopics = [
  "rawtx",
  "rawblock",
  "rawgovernancevote",
];

let messages = [];
let listenerAdded = {};

// ajquery - still great after all these years!
function $(sel, el) {
  return (el || document).querySelector(sel);
}

function $$(sel, el) {
  return Array.from((el || document).querySelectorAll(sel));
}

async function request(topic, payload) {
  let method = "GET";
  let headers = {};
  if (payload) {
    method = "POST";
    Object.assign(headers, {
      "Content-Type": "application/json",
    });
  }
  let resp = await fetch(`${baseUrl}${topic}`, {
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
    return { topics: [] };
  }

  let queryIter = new URLSearchParams(fragment);
  let query = Object.fromEntries(queryIter);

  let topics;
  let topicList = query.topics || "";
  topicList = topicList.trim();
  if (topicList) {
    topics = topicList.split(/[,\s]/);
  } else {
    topics = [];
  }

  return {
    topics: topics,
  };
}

/**
 * @param {Event}
 */
App.$setSubscriptions = async function ($ev) {
  console.info(`Topic Change: Set Subscriptions`);
  let topic = $ev.target.value.trim();
  if (!topic) {
    return false;
  }

  let isKnown = allowedTopics.includes(topic);
  if (!isKnown) {
    return false;
    // throw new Error(`topic '${topic}' is not known or not allowed`);
  }

  let visibilityByTopic = App._$getTopicsMap();
  let exists = topic in visibilityByTopic;
  visibilityByTopic[topic] = true;
  App.$renderTags(visibilityByTopic);
  $ev.target.value = '';

  if (exists) {
    return
  }

  await App.$updatePreview();
  await App.submitForm();
};

/**
 * @param {Event}
 */
App.$unsetTopic = async function ($ev) {
  let oldTopic = $ev.target.closest('.tag').querySelector('input').value;
  console.log(`DEBUG DELETE`, oldTopic, $ev.target);

  let visibilityByTopic = App._$getTopicsMap();
  visibilityByTopic[oldTopic] = null;
  delete visibilityByTopic[oldTopic];
  App.$renderTags(visibilityByTopic);

  await App.$updatePreview();
  await App.submitForm();
};


App.$renderTags = function (tagsMap) {
	let $container = $(`[data-id="tagContainer"]`);
	$container.innerHTML = '';

	let $template = $('[data-id="tmplTopicTag"').content;
    let $tagsFragment = new DocumentFragment();
    let tags = Object.keys(tagsMap);
    tags.sort();
	for (let tag of tags) {
        let checked = tagsMap[tag];
		let $tagItem = $template.cloneNode(true);
		let $checkbox = $tagItem.querySelector('input[type="checkbox"]');
		let $label = $tagItem.querySelector('label span');

		$checkbox.value = tag;
		$checkbox.checked = checked;
        $label.textContent = tag;

		$tagsFragment.appendChild($tagItem);
	}
	$container.replaceChildren($tagsFragment);

    let optionList = [];
    for (let topic of allowedTopics) {
      if (!tagsMap[topic]) {
        optionList.push(`<option value="${topic}">${topic}</option>`);
      }
    }
    let options = optionList.join("\n");

    let $zmqwebproxyTopics = document.querySelector("#topics");
    $zmqwebproxyTopics.innerText = "";
    $zmqwebproxyTopics.insertAdjacentHTML("afterbegin", options);
}

App.$renderMessages = function (event) {
  let visibilityByTopic = App._$getTopicsMap();
  let $messages = $('[data-id="output"]')
  $messages.textContent = '';
  for (let message of messages) {
    if (!visibilityByTopic[message.event]) {
        if (message.event === "default") {
            $('[data-id="output"]').insertAdjacentText('afterbegin', `(${message.event}) ${message.data}\n`);
        }
        continue;
    }

    $('[data-id="output"]').insertAdjacentText('afterbegin', `[${message.event}]\n${message.data}\n\n`);
  }
}

App._$getTopicsMap = function () {
  let topics = {};
  let $topics = $$("input[name=topic]");
  for (let $topic of $topics) {
    let isKnown = allowedTopics.includes($topic.value);
    if (isKnown) {
      topics[$topic.value] = $topic.checked;
    }
  }

  return topics;
}

App.$updatePreview = async function (event) {
  let defaults = { port: "9998" };
  if (network === "testnet") {
    defaults = {
      port: "19998",
    };
  }

  let visibilityByTopic = App._$getTopicsMap();
  let topics = Object.keys(visibilityByTopic);
  topics.sort();
  let topicList = topics.join(',');
  topicList = topicList.replace(',debug:ticker,', ',');
  topicList = topicList.replace(/,?debug:ticker,?/, '');

  let shareHash = `#?topics=${topicList}`;
  let shareUrl = `${location.protocol}//${location.host}/${shareHash}`;
  $("[data-id=share]").href = shareUrl;
  $("[data-id=share]").innerText = shareUrl;

  let opts = { topics: topics };
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

  await App.submitForm();
};

App.submitForm = async function () {
  let visibilityByTopic = App._$getTopicsMap();
  let topics = Object.keys(visibilityByTopic);
  console.info(`Form Submit: Set Subscriptions:`, visibilityByTopic);

  let result = await setSubscriptions(topics).catch(function (err) {
    console.error(`Form Submit: Failed`);
    console.error(err);
    let data = {
      code: err.code,
      message: err.message,
    };
    return data;
  });
  let json = JSON.stringify(result, null, 2);

  for (let topic of topics) {
    let fn = listenerAdded[topic];
    if (fn) {
      eventSource.removeEventListener(topic, fn);
      eventSource.addEventListener(topic, fn);
      continue;
    }

    listenerAdded[topic] = function ($ev) {
        let data = $ev.data;
        if (topic !== DEBUG_TICKER) {
          try {
              let json = JSON.parse(data);
              data = JSON.stringify(json, null, 2);
          } catch(e) {
              // ignore non-json
          }
        }

        let event = { id: $ev.lastEventId, event: topic, data: data};
        messages.push(event);

        let visibilityByTopic = App._$getTopicsMap();
        if (visibilityByTopic[topic]) {
          $('[data-id="output"]').insertAdjacentText('afterbegin', `[${event.event}]\n${event.data}\n\n`);
        }
    };
    eventSource.addEventListener(topic, listenerAdded[topic]);
  }

  // TODO set status
  // $("[data-id=output]").textContent += json;
};

async function setSubscriptions(topics) {
  let resp = await fetch(`/api/zmq/eventsource/${eventSourceId}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      topics: topics
    }),
  });
  if (!resp.ok) {
    let msg = await resp.text();
    throw new Error(msg);
  }

  let data = await resp.json();
  console.info(`Set subscriptions:`, data);
}

function renderCurl(opts) {
  let topics = opts.topics.slice(0);
  topics.sort();
  let topicList = topics.join(',');
  let code = `

curl --fail-with-body -N -G \\
    "https://${window.location.host}/api/zmq/eventsource/$(uuidgen)" \\
    --user "api:null" \\
    -d 'dbg_topics=${topicList}'
`;
  code = code.trim();
  return code;
}

function renderFetch(opts) {
  let topics = opts.topics.slice(0);
  topics.sort();
  let topicList = topics.join('", "');
  let code = `

// 1. Open EventSource
let sseId = crypto.randomUUID();
let baseUrl = \`https://${window.location.host}/api/zmq/eventsource/\${sseId}\`;
let sse = new EventSource(baseUrl, { withCredentials: false });

// 2. Listen on local Events
let topics = ["${topicList}"];
for (let topic of topics) {
    sse.addEventListener(topic, function ($ev) {
        console.info(\`[\${topic}] \${$ev.data}\`);
    });
}

// Subscribe to remote Topics
let basicAuth = btoa(\`api:null\`);
let resp = await fetch(baseUrl, {
    method: 'PUT',
    headers: {
        "Authorization": \`Basic \${basicAuth}\`,
        "Content-Type": "application/json",
    },
    body: JSON.stringify({ topics: topics }),
});
let result = await resp.text();
console.log(\`[DEBUG] status: \${result}\`);

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
    allowedTopics = [];
    for (let topic of data.topics) {
      // ... include, exclude, traverse here
      let isComment = /^([/][/]|[/][*]|#)/.test(topic)
      if (isComment) {
        continue
      }
      allowedTopics.push(topic);
    }
  }

  let opts = parseHashQuery(location.hash);
  {
    if (!opts.topics.length) {
      for (let topic of defaultTopics) {
        let isKnown = allowedTopics.includes(topic);
        if (!isKnown) {
          continue;
        }
        opts.topics.push(topic);
      }
    }

    let hasDebugTicker = opts.topics.includes(DEBUG_TICKER);
    if (!hasDebugTicker) {
      let isKnown = allowedTopics.includes(DEBUG_TICKER);
      if (isKnown) {
        opts.topics.push(DEBUG_TICKER)
      }
    }
  }

  let visibilityByTopic = {};
  {
    for (let topic of opts.topics) {
      visibilityByTopic[topic] = true;
    }
    App.$renderTags(visibilityByTopic);
  }

  console.log('DEBUG parse topics', opts.topics);
  document.body.removeAttribute("hidden");

  // TODO debounce immediate failures
  function initEventSource() {
    eventSource = new EventSource(`/api/zmq/eventsource/${eventSourceId}`, { withAuthentication: false });
    eventSource.onerror = function (_) {
      if (isUnloading) {
        return;
      }
      eventSource.close();
      initEventSource();

      throw new Error(`EventSource restarted unexpectedly (check the NETWORK console for error)`);
    };
    eventSource.onopen = async function ($ev) {
      let topics = Object.keys(visibilityByTopic);
      await App.submitForm();
    };
    eventSource.onmessage = function ($ev) {
      let event = { id: $ev.lastEventId, event: "default", data: $ev.data};
      messages.push(event);
      $('[data-id="output"]').insertAdjacentText('afterbegin', `(${event.event}) ${event.data}\n\n`);
    };
  }
  initEventSource();

  await App.$updatePreview();

  if (opts?.submit && opts?.topic) {
    let isKnown = allowedTopics.includes(opts.topic);
    if (!isKnown) {
      return;
    }

    let event = new Event("submit", {
      bubbles: true,
      cancelable: true,
    });
    $("form#form").dispatchEvent(event);
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

window.onbeforeunload = function (ev) {
  isUnloading = true;
};

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
