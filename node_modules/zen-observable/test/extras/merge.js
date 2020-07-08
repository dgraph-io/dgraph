import assert from 'assert';
import { parse } from './parse.js';
import { merge } from '../../src/extras.js';

describe('extras/merge', () => {
  it('should emit all data from each input in parallel', async () => {
    let output = '';
    await merge(
      parse('a-b-c-d'),
      parse('-A-B-C-D')
    ).forEach(
      value => output += value
    );
    assert.equal(output, 'aAbBcCdD');
  });
});
