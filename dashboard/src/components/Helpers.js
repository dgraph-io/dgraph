export function checkStatus(response) {
  if (response.status >= 200 && response.status < 300) {
    return response;
  } else {
    let error = new Error(response.statusText);
    error["response"] = response;
    throw error;
  }
}

export function parseJSON(response) {
  return response.json();
}

export function timeout(ms, promise) {
  return new Promise(function(resolve, reject) {
    setTimeout(
      function() {
        reject(new Error("timeout"));
      },
      ms,
    );
    promise.then(resolve, reject);
  });
}

export function aggregationPrefix(properties) {
  let aggTerms = ["count", "max(", "min(", "sum("];
  for (let k in properties) {
    if (!properties.hasOwnProperty(k)) {
      continue;
    }
    for (let term of aggTerms) {
      if (k.startsWith(term)) {
        if (term === "count") {
          return term;
        }
        return term.substr(0, term.length - 1);
      }
    }
  }
  return "";
}

function getNameKey(properties) {
  for (let i in properties) {
    if (!properties.hasOwnProperty(i)) {
      continue;
    }
    let toLower = i.toLowerCase();
    if (toLower === "name" || toLower === "name.en") {
      return i;
    }
  }
  return "";
}

export function getNodeLabel(properties: Object): string {
  var label = "";

  let keys = Object.keys(properties);
  if (keys.length === 1) {
    label = aggregationPrefix(properties);
    if (label !== "") {
      return label;
    }
  }

  let nameKey = getNameKey(properties);
  if (nameKey === "") {
    return "";
  }

  label = properties[nameKey];

  var words = label.split(" "), firstWord = words[0];
  if (firstWord.length > 20) {
    label = [firstWord.substr(0, 9), firstWord.substr(9, 17) + "..."].join(
      "-\n",
    );
  } else if (firstWord.length > 10) {
    label = [
      firstWord.substr(0, 9),
      firstWord.substr(9, firstWord.length),
    ].join("-\n");
  } else {
    // First word is less than 10 chars so we can display it in full.
    if (words.length > 1) {
      if (words[1].length > 10) {
        label = [firstWord, words[1] + "..."].join("\n");
      } else {
        label = [firstWord, words[1]].join("\n");
      }
    } else {
      label = firstWord;
    }
  }

  return label;
}

export function outgoingEdges(nodeId, edgeSet) {
  return edgeSet.get({
    filter: function(edge) {
      return edge.from === nodeId;
    },
  });
}

export function isShortestPath(query) {
  return query.indexOf("shortest") !== -1 &&
    query.indexOf("to") !== -1 &&
    query.indexOf("from") !== -1;
}
