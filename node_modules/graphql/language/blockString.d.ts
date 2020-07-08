/**
 * Produces the value of a block string from its parsed raw value, similar to
 * CoffeeScript's block string, Python's docstring trim or Ruby's strip_heredoc.
 *
 * This implements the GraphQL spec's BlockStringValue() static algorithm.
 */
export function dedentBlockStringValue(rawString: string): string;

/**
 * @internal
 */
export function getBlockStringIndentation(lines: ReadonlyArray<string>): number;

/**
 * Print a block string in the indented block form by adding a leading and
 * trailing blank line. However, if a block string starts with whitespace and is
 * a single-line, adding a leading blank line would strip that whitespace.
 */
export function printBlockString(
  value: string,
  indentation?: string,
  preferMultipleLines?: boolean,
): string;
