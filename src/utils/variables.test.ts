import { format } from './variables';

describe('format', () => {
  it('should return a single value as is', () => {
    expect(format('test')).toBe('test');
  });

  // it('should join array values with commas', () => {
  //   expect(format(['value1', 'value2', 'value3'])).toBe("'value1','value2','value3'");
  // });

  // it('should return empty string for empty array values', () => {
  //   expect(format([])).toBe("''");
  // });

  // it('should return undefined for undefined values', () => {
  //   expect(format(undefined)).toBeUndefined();
  // });
});
