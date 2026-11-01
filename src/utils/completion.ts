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
/**
 * Matches the FROM target only: an optionally quoted, optionally dotted
 * identifier. Subqueries and comma-joined tables are deliberately not matched,
 * because rewriting those silently would change the query's meaning.
 */
const FROM_TARGET = /\b(from)\s+("(?:[^"]|"")*"|`[^`]*`|[A-Za-z_][\w.]*)/i;

/** Reads the FROM target of a statement, if any. */
export function extractFromTarget(sql: string): { table: string; start: number; end: number } | undefined {
  const match = FROM_TARGET.exec(sql);
  if (!match) {
    return undefined;
  }
  const raw = match[2];
  const table = raw.startsWith('"') ? raw.slice(1, -1).replace(/""/g, '"') : raw.startsWith('`') ? raw.slice(1, -1) : raw;
  const start = match.index + match[0].length - raw.length;
  return { table, start, end: start + raw.length };
}

/**
 * Rewrites only the stream named after FROM, leaving the rest of the statement
 * untouched.
 *
 * This is what the stream picker uses: picking a stream must not discard a
 * WHERE clause, a projection or a LIMIT the user wrote by hand. When there is
 * no FROM clause yet, a minimal SELECT is seeded.
 */
export function retargetStream(sql: string, table: string): string {
  const quoted = `"${table.replace(/"/g, '""')}"`;
  const target = extractFromTarget(sql);
  if (!target) {
    const trimmed = sql.trim();
    return trimmed === '' ? `SELECT * FROM ${quoted} LIMIT 100` : `${trimmed} FROM ${quoted}`;
  }
  return sql.slice(0, target.start) + quoted + sql.slice(target.end);
}

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
