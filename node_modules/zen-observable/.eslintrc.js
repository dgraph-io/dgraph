module.exports = {
  "extends": ["eslint:recommended"],

  "env": {
    "es6": true,
    "node": true
  },

  "globals": {
    "setTimeout": true
  },

  "parserOptions": {
    "sourceType": "module"
  },

  "rules": {
    "no-console": ["error", { "allow": ["warn", "error"] }],
    "no-unsafe-finally": ["off"],
    "camelcase": ["error", { "properties": "always" }],
    "brace-style": ["off"],
    "eqeqeq": ["error", "smart"],
    "indent": ["error", 2, { "SwitchCase": 1 }],
    "no-throw-literal": ["error"],
    "comma-spacing": ["error", { "before": false, "after": true }],
    "comma-style": ["error", "last"],
    "comma-dangle": ["error", "always-multiline"],
    "keyword-spacing": ["error"],
    "no-trailing-spaces": ["error"],
    "no-multi-spaces": ["error"],
    "no-spaced-func": ["error"],
    "no-whitespace-before-property": ["error"],
    "space-before-blocks": ["error"],
    "space-before-function-paren": ["error", "never"],
    "space-in-parens": ["error", "never"],
    "eol-last": ["error"],
    "quotes": ["error", "single", { "avoidEscape": true }],
    "no-implicit-globals": ["error"],
    "no-useless-concat": ["error"],
    "space-infix-ops": ["error", { "int32Hint": true }],
    "semi-spacing": ["error", { "before": false, "after": true }],
    "semi": ["error", "always", { "omitLastInOneLineBlock": true }],
    "object-curly-spacing": ["error", "always"],
    "array-bracket-spacing": ["error"],
    "max-len": ["error", 100]
  }
};
