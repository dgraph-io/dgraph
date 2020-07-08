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
  input: "src/context.ts",
  external,
  output: {
    file: "lib/context.esm.js",
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
  input: "lib/context.esm.js",
  external,
  output: {
    // Intentionally overwrite the context.js file written by tsc:
    file: "lib/context.js",
    format: "cjs",
    exports: "named",
    sourceMap: true,
    name: "context",
    globals,
  },
}];
