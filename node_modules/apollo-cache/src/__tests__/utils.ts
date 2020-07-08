import { print } from 'graphql/language/printer';
import { queryFromPojo, fragmentFromPojo } from '../utils';

describe('writing data with no query', () => {
  describe('converts a JavaScript object to a query correctly', () => {
    it('basic', () => {
      expect(
        print(
          queryFromPojo({
            number: 5,
            bool: true,
            bool2: false,
            undef: undefined,
            nullField: null,
            str: 'string',
          }),
        ),
      ).toMatchSnapshot();
    });

    it('nested', () => {
      expect(
        print(
          queryFromPojo({
            number: 5,
            bool: true,
            nested: {
              bool2: false,
              undef: undefined,
              nullField: null,
              str: 'string',
            },
          }),
        ),
      ).toMatchSnapshot();
    });

    it('arrays', () => {
      expect(
        print(
          queryFromPojo({
            number: [5],
            bool: [[true]],
            nested: [
              {
                bool2: false,
                undef: undefined,
                nullField: null,
                str: 'string',
              },
            ],
          }),
        ),
      ).toMatchSnapshot();
    });

    it('fragments', () => {
      expect(
        print(
          fragmentFromPojo({
            number: [5],
            bool: [[true]],
            nested: [
              {
                bool2: false,
                undef: undefined,
                nullField: null,
                str: 'string',
              },
            ],
          }),
        ),
      ).toMatchSnapshot();
    });
  });
});
