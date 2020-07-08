declare module "graphql-tag" {
  function gql(
    literals: ReadonlyArray<string> | Readonly<string>,
    ...placeholders: any[]
  ): import("graphql").DocumentNode;

  namespace gql {
    function resetCaches(): void;
    function disableFragmentWarnings(): void;
    function enableExperimentalFragmentVariables(): void;
    function disableExperimentalFragmentVariables(): void;
  }

  export default gql;

  export function resetCaches(): void;
  export function disableFragmentWarnings(): void;
  export function enableExperimentalFragmentVariables(): void;
  export function disableExperimentalFragmentVariables(): void;
}
