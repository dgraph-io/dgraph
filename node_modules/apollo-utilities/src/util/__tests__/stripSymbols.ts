import { stripSymbols } from '../stripSymbols';

interface SymbolConstructor {
  (description?: string | number): symbol;
}

declare const Symbol: SymbolConstructor;

describe('stripSymbols', () => {
  it('should strip symbols (only)', () => {
    const sym = Symbol('id');
    const data = { foo: 'bar', [sym]: 'ROOT_QUERY' };
    expect(stripSymbols(data)).toEqual({ foo: 'bar' });
  });
});
