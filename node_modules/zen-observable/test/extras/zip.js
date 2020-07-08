import assert from 'assert';
import { parse } from './parse.js';
import { zip } from '../../src/extras.js';

describe('extras/zip', () => {
  it('should emit pairs of corresponding index values', async () => {
    let output = [];
    await zip(
      parse('a-b-c-d'),
      parse('-A-B-C-D')
    ).forEach(
      value => output.push(value.join(''))
    );
    assert.deepEqual(output, [
      'aA',
      'bB',
      'cC',
      'dD',
    ]);
  });
});
