import { extractFirstTable } from './completion';

describe('extractFirstTable', () => {
  it('reads a double quoted stream name', () => {
    expect(extractFirstTable('SELECT * FROM "vnet_bff_dev" LIMIT 10')).toBe('vnet_bff_dev');
  });

  it('reads a backtick quoted stream name', () => {
    expect(extractFirstTable('select * from `my_stream`')).toBe('my_stream');
  });

  it('reads a bare identifier', () => {
    expect(extractFirstTable('SELECT body FROM logs')).toBe('logs');
  });

  it('accepts a dotted identifier', () => {
    expect(extractFirstTable('SELECT * FROM default.app_logs')).toBe('default.app_logs');
  });

  it('ignores the case of the keywords', () => {
    expect(extractFirstTable('SeLeCt * FrOm "x"')).toBe('x');
  });

  it('returns undefined without a FROM clause', () => {
    expect(extractFirstTable('SELECT 1')).toBeUndefined();
    expect(extractFirstTable('')).toBeUndefined();
  });

  it('does not match a column called from', () => {
    // "from" only counts as a keyword when it is a standalone word.
    expect(extractFirstTable('SELECT from_address FROM "t"')).toBe('t');
  });
});
