import { OpenObserveDataSource } from '../datasource';
import { OpenObserveQuery } from '../types';

/** The shape @grafana/ui's CodeEditor expects for a completion entry. */
export interface Suggestion {
  label: string;
  kind?: string;
  detail?: string;
  documentation?: string;
  insertText?: string;
}

/** SQL keywords offered by the OpenObserve SQL completion. */
const KEYWORDS = [
  'SELECT',
  'FROM',
  'WHERE',
  'GROUP BY',
  'ORDER BY',
  'LIMIT',
  'OFFSET',
  'AS',
  'AND',
  'OR',
  'NOT',
  'IN',
  'LIKE',
  'BETWEEN',
  'IS NULL',
  'IS NOT NULL',
  'HAVING',
  'DISTINCT',
  'ASC',
  'DESC',
];

/** OpenObserve SQL functions worth completing. */
const FUNCTIONS = [
  'count',
  'sum',
  'avg',
  'min',
  'max',
  'approx_distinct',
  'median',
  'histogram',
  'date_trunc',
  'to_timestamp',
  'to_string',
  'to_int',
  'str_match',
  'regexp_like',
  'coalesce',
  'ifnull',
  'nullif',
  'round',
  'floor',
  'ceil',
];

/**
 * Fetches columns + keywords for the stream referenced by the query.
 *
 * The SQL editor calls this on every keystroke, so the datasource caches the
 * stream list per stream type.
 */
export async function completionFetchFields(
  datasource: OpenObserveDataSource,
  query: Partial<OpenObserveQuery>
): Promise<Suggestion[]> {
  const suggestions: Suggestion[] = [];

  const streamType = query.queryType ?? 'logs';
  const table = query.table ?? extractFirstTable(query.rawSql ?? '');

  if (table) {
    try {
      const fields = await datasource.fetchFields(streamType, table);
      for (const field of fields) {
        suggestions.push({
          label: field.name,
          kind: 'field',
          detail: `${table}.${field.name}`,
          insertText: field.name,
        });
      }
    } catch {
      // Completion must never break typing; fall through to the keywords.
    }
  }

  for (const name of FUNCTIONS) {
    suggestions.push({ label: name, kind: 'function', insertText: `${name}(` });
  }
  for (const keyword of KEYWORDS) {
    suggestions.push({ label: keyword, kind: 'keyword', insertText: keyword });
  }
  return suggestions;
}

/** Stream names of the selected type, quoted for SQL. */
export async function completionFetchTables(
  datasource: OpenObserveDataSource,
  streamType: string
): Promise<Suggestion[]> {
  try {
    const streams = await datasource.listStreams(streamType);
    return streams.map(name => ({ label: name, kind: 'table', insertText: `"${name}"` }));
  } catch {
    return [];
  }
}

/**
 * extractFirstTable pulls the stream name out of "FROM <ident>" so completion
 * can suggest that stream's columns even before the user picks it explicitly.
 */
export function extractFirstTable(sql: string): string | undefined {
  const match = /\bfrom\s+("([^"]+)"|`([^`]+)`|([A-Za-z_][\w.]*))/i.exec(sql);
  if (!match) {
    return undefined;
  }
  return match[2] ?? match[3] ?? match[4];
}

/** Metric names for PromQL completion. */
export async function completionFetchMetrics(datasource: OpenObserveDataSource): Promise<Suggestion[]> {
  try {
    const names = await datasource.listMetricNames();
    return names.map(name => ({ label: name, kind: 'metric', insertText: name }));
  } catch {
    return [];
  }
}
