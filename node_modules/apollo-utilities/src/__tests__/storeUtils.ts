import { getStoreKeyName } from '../storeUtils';

describe('getStoreKeyName', () => {
  it(
    'should return a deterministic version of the store key name no matter ' +
      'which order the args object properties are in',
    () => {
      const validStoreKeyName =
        'someField({"prop1":"value1","prop2":"value2"})';
      let generatedStoreKeyName = getStoreKeyName('someField', {
        prop1: 'value1',
        prop2: 'value2',
      });
      expect(generatedStoreKeyName).toEqual(validStoreKeyName);

      generatedStoreKeyName = getStoreKeyName('someField', {
        prop2: 'value2',
        prop1: 'value1',
      });
      expect(generatedStoreKeyName).toEqual(validStoreKeyName);
    },
  );
});
