import { AdHocVariableFilter, DataSourceJsonData } from '@grafana/data';

/** The OpenObserve stream type a query targets. */
export type StreamType = 'logs' | 'metrics' | 'traces';

/** How the query is expressed. */
export type QueryMode = 'sql' | 'promql';

/** Trace editor modes, mirroring Jaeger's three query types. */
export type TraceMode = 'search' | 'traceId' | 'graph';

/** The shape the backend turns the response into. */
export type ResultFormat = 'auto' | 'logs' | 'table' | 'timeseries' | 'trace' | 'annotations';

/**
 * How the editor expresses the query.
 *
 * `builder` uses structured fields and the backend derives the SQL from them;
 * `code` is the statement the user wrote. The two never write into each other,
 * so switching modes cannot clobber work.
 */
export type EditorMode = 'builder' | 'code';

/** Aggregations the builder offers. */
export type BuilderAggregation = 'avg' | 'sum' | 'min' | 'max' | 'count' | 'p50' | 'p95' | 'p99';

/**
 * Represents a query specific to the OpenObserve data source.
 */
export interface OpenObserveQuery {
  refId: string;
  hide?: boolean;

  /** OpenObserve stream type: logs, metrics or traces. */
  queryType: StreamType;
  /** SQL or PromQL. */
  queryMode?: QueryMode;

  /** The SQL statement (queryMode = sql). */
  rawSql?: string;
  /** The PromQL expression (queryMode = promql). */
  expr?: string;

  /** Frame shape requested from the backend. */
  format?: ResultFormat;
  /** builder (structured fields) or code (hand-written SQL). */
  editorMode?: EditorMode;

  /** Builder fields; only read when editorMode is "builder". */
  builderStream?: string;
  builderAggregation?: BuilderAggregation | '';
  builderField?: string;
  builderInterval?: string;
  builderGroupBy?: string[];

  /** Row limit override. 0 means "derive from LIMIT / datasource default". */
  size?: number;
  /** Row offset for pagination. */
  from?: number;

  /** Use the SSE streaming search endpoint. */
  enableSSE?: boolean;
  /** Allow OpenObserve to serve the query from its result cache. */
  useCache?: boolean;

  /** OpenObserve search type: ui, dashboards, reports or alerts. */
  searchType?: string;

  /** Restrict the trace view to a single trace. */
  traceId?: string;

  /** Trace editor mode: search (list), traceId (one trace), graph (services). */
  traceMode?: TraceMode;
  /** Trace search filters. */
  serviceName?: string;
  operationName?: string;
  traceTags?: string[];
  minDuration?: string;
  maxDuration?: string;

  /** Per-query overrides of the datasource column mapping. */
  timeField?: string;
  logMessageField?: string;
  logLevelField?: string;

  /** Ad-hoc variable filters injected into the SQL. */
  adhocFilters?: AdHocVariableFilter[];

  /** Set by variable queries. */
  rawQuery?: boolean;

  /** The stream picked in the editor; used for completion and variables. */
  table?: string;
}

/**
 * Options configured for each DataSource instance.
 */
export interface OpenObserveOptions extends DataSourceJsonData {
  url: string;
  user?: string;
  /** OpenObserve organization (kept as "database" for backwards compatibility). */
  database?: string;
  organization?: string;
  authMethod?: 'basic' | 'token';
  tlsSkipVerify?: boolean;
  timeoutSeconds?: number;
  defaultLimit?: number;
  maxLimit?: number;
  timeField?: string;
  logMessageField?: string;
  logLevelField?: string;
  enablePrometheus?: boolean;
}

/**
 * Value that is used in the backend, but never sent over HTTP to the frontend.
 */
export interface OpenObserveSecureJsonData {
  password?: string;
  token?: string;
}

/** One stream with its column names, as returned by /openobserve/streams. */
export type StreamColumns = Record<string, string[]>;

/** One column of a stream schema. */
export interface SchemaField {
  name: string;
  type: string;
}

export const DEFAULT_QUERY: Partial<OpenObserveQuery> = {
  queryType: 'logs',
  queryMode: 'sql',
  format: 'auto',
  // SSE and cache are on by default and have no UI: they are transport
  // details, not query semantics, and the toggles only invited mistakes.
  enableSSE: true,
  useCache: true,
  size: 100,
};

/** Row limit used when a query does not set one. */
export const DEFAULT_LIMIT = 100;

/** Default SQL shown for a brand new panel. */
export const DEFAULT_SQL = 'SELECT * FROM "your_stream" LIMIT 100';

/** The stream types offered in the editor. */
export const STREAM_TYPE_OPTIONS = [
  { label: 'Logs', value: 'logs' as StreamType },
  { label: 'Metrics', value: 'metrics' as StreamType },
  { label: 'Traces', value: 'traces' as StreamType },
];

/** The result formats offered in the editor. */
export const FORMAT_OPTIONS = [
  { label: 'Auto', value: 'auto' as ResultFormat },
  { label: 'Logs', value: 'logs' as ResultFormat },
  { label: 'Table', value: 'table' as ResultFormat },
  { label: 'Time series', value: 'timeseries' as ResultFormat },
  { label: 'Trace', value: 'trace' as ResultFormat },
  { label: 'Annotations', value: 'annotations' as ResultFormat },
];
