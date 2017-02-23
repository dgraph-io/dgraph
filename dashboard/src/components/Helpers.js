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
