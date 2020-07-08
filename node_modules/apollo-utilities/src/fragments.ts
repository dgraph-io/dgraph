import { DocumentNode, FragmentDefinitionNode } from 'graphql';
import { invariant, InvariantError } from 'ts-invariant';

/**
 * Returns a query document which adds a single query operation that only
 * spreads the target fragment inside of it.
 *
 * So for example a document of:
 *
 * ```graphql
 * fragment foo on Foo { a b c }
 * ```
 *
 * Turns into:
 *
 * ```graphql
 * { ...foo }
 *
 * fragment foo on Foo { a b c }
 * ```
 *
 * The target fragment will either be the only fragment in the document, or a
 * fragment specified by the provided `fragmentName`. If there is more than one
 * fragment, but a `fragmentName` was not defined then an error will be thrown.
 */
export function getFragmentQueryDocument(
  document: DocumentNode,
  fragmentName?: string,
): DocumentNode {
  let actualFragmentName = fragmentName;

  // Build an array of all our fragment definitions that will be used for
  // validations. We also do some validations on the other definitions in the
  // document while building this list.
  const fragments: Array<FragmentDefinitionNode> = [];
  document.definitions.forEach(definition => {
    // Throw an error if we encounter an operation definition because we will
    // define our own operation definition later on.
    if (definition.kind === 'OperationDefinition') {
      throw new InvariantError(
        `Found a ${definition.operation} operation${
          definition.name ? ` named '${definition.name.value}'` : ''
        }. ` +
          'No operations are allowed when using a fragment as a query. Only fragments are allowed.',
      );
    }
    // Add our definition to the fragments array if it is a fragment
    // definition.
    if (definition.kind === 'FragmentDefinition') {
      fragments.push(definition);
    }
  });

  // If the user did not give us a fragment name then let us try to get a
  // name from a single fragment in the definition.
  if (typeof actualFragmentName === 'undefined') {
    invariant(
      fragments.length === 1,
      `Found ${
        fragments.length
      } fragments. \`fragmentName\` must be provided when there is not exactly 1 fragment.`,
    );
    actualFragmentName = fragments[0].name.value;
  }

  // Generate a query document with an operation that simply spreads the
  // fragment inside of it.
  const query: DocumentNode = {
    ...document,
    definitions: [
      {
        kind: 'OperationDefinition',
        operation: 'query',
        selectionSet: {
          kind: 'SelectionSet',
          selections: [
            {
              kind: 'FragmentSpread',
              name: {
                kind: 'Name',
                value: actualFragmentName,
              },
            },
          ],
        },
      },
      ...document.definitions,
    ],
  };

  return query;
}
