import { format, quote } from './variables';

describe('format', () => {
  it('returns a plain string quoted', () => {
    expect(format('test')).toBe("'test'");
  });

  it('joins array values as a quoted list', () => {
    expect(format(['a', 'b', 'c'])).toBe("'a','b','c'");
  });

  it('keeps numeric arrays unquoted', () => {
    expect(format([1, 2, 3])).toBe('1,2,3');
  });

  it('mixes quoted strings and bare numbers', () => {
    expect(format(['a', 2])).toBe("'a',2");
  });

  it('returns NULL for an empty array', () => {
    expect(format([])).toBe('NULL');
  });

  it('returns NULL for null and undefined', () => {
    expect(format(null)).toBe('NULL');
    expect(format(undefined)).toBe('NULL');
  });

  it('emits numbers and booleans unquoted', () => {
    expect(format(42)).toBe('42');
    expect(format(true)).toBe('true');
  });

  it('escapes single quotes', () => {
    expect(format("it's")).toBe("'it''s'");
    expect(quote("a'b")).toBe("'a''b'");
  });
});
