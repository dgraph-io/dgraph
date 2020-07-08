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
  input: "src/equality.ts",
  external,
  output: {
    file: "lib/equality.esm.js",
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
  input: "lib/equality.esm.js",
  external,
  output: {
    // Intentionally overwrite the equality.js file written by tsc:
    file: "lib/equality.js",
    format: "cjs",
    exports: "named",
    sourcemap: true,
    name: "equality",
    globals,
  },
}];
