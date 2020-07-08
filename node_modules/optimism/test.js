describe("Compiled to CommonJS", function () {
  require("./lib/tests/bundle.cjs.js");
});

describe("Compiled to ECMAScript module syntax", function () {
  require("reify/node"); // Enable ESM syntax.
  require("./lib/tests/bundle.esm.js");
});
