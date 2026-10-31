import { ScopedVars } from '@grafana/data';
import { getTemplateSrv } from '@grafana/runtime';

/**
 * Interpolates Grafana template variables in a SQL statement.
 */
export const replace = (value?: string, scopedVars?: ScopedVars) => {
  if (value === undefined) {
    return value;
  }
  return getTemplateSrv().replace(value, scopedVars, format);
};

/**
 * Formats a template variable value for an OpenObserve SQL statement.
 *
 *  - arrays become `'a','b'` (quoted) or `1,2` (all numeric)
 *  - single quotes are escaped for the OpenObserve dialect (`''`)
 *  - null/undefined become the SQL keyword NULL
 */
export const format = (value: unknown): string => {
  if (value === null || value === undefined) {
    return 'NULL';
  }
  if (Array.isArray(value)) {
    if (value.length === 0) {
      return 'NULL';
    }
    return value.map(v => (typeof v === 'number' ? String(v) : quote(v))).join(',');
  }
  if (typeof value === 'number') {
    return String(value);
  }
  if (typeof value === 'boolean') {
    return value ? 'true' : 'false';
  }
  return quote(value);
};

/** Escapes a value for use inside a single-quoted SQL literal. */
export const quote = (value: unknown): string => `'${String(value).replace(/'/g, "''")}'`;
