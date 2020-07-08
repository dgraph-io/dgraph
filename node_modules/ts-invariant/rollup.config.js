import typescriptPlugin from 'rollup-plugin-typescript2';
import typescript from 'typescript';

const globals = {
  __proto__: null,
  tslib: "tslib",
};

function external(id) {
  return id in globals;
}

export default [{
  input: "src/invariant.ts",
  external,
  output: {
    file: "lib/invariant.esm.js",
    format: "esm",
    sourcemap: true,
    globals,
  },
  plugins: [
    typescriptPlugin({
      typescript,
      tsconfig: "./tsconfig.rollup.json",
    }),
  ],
}, {
  input: "lib/invariant.esm.js",
  external,
  output: {
    // Intentionally overwrite the invariant.js file written by tsc:
    file: "lib/invariant.js",
    format: "cjs",
    exports: "named",
    sourcemap: true,
    name: "ts-invariant",
    globals,
  },
}];
