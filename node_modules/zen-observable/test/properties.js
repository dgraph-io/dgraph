import assert from 'assert';

export function testMethodProperty(object, key, options) {
  let desc = Object.getOwnPropertyDescriptor(object, key);
  let { enumerable = false, configurable = false, writable = false, length } = options;

  assert.ok(desc, `Property ${ key.toString() } exists`);

  if (options.get || options.set) {
    if (options.get) {
      assert.equal(typeof desc.get, 'function', 'Getter is a function');
      assert.equal(desc.get.length, 0, 'Getter length is 0');
    } else {
      assert.equal(desc.get, undefined, 'Getter is undefined');
    }

    if (options.set) {
      assert.equal(typeof desc.set, 'function', 'Setter is a function');
      assert.equal(desc.set.length, 1, 'Setter length is 1');
    } else {
      assert.equal(desc.set, undefined, 'Setter is undefined');
    }
  } else {
    assert.equal(typeof desc.value, 'function', 'Value is a function');
    assert.equal(desc.value.length, length, `Function length is ${ length }`);
    assert.equal(desc.writable, writable, `Writable property is correct ${ writable }`);
  }

  assert.equal(desc.enumerable, enumerable, `Enumerable property is ${ enumerable }`);
  assert.equal(desc.configurable, configurable, `Configurable property is ${ configurable }`);
}
