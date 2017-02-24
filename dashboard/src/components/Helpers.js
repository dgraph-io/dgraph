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

export function getNodeLabel(properties: Object): string {
  var label = "";
  if (properties["name"] !== undefined) {
    label = properties["name"];
  } else if (properties["name.en"] !== undefined) {
    label = properties["name.en"];
  } else {
    return "";
  }

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
  console.log("nodeId", nodeId, "edgeSet", edgeSet);
  return edgeSet.get({
    filter: function(edge) {
      return edge.from === nodeId;
    },
  });
}
