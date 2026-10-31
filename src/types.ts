import { AdHocVariableFilter, DataSourceJsonData } from '@grafana/data';

/** The OpenObserve stream type a query targets. */
export type StreamType = 'logs' | 'metrics' | 'traces';

/** How the query is expressed. */
export type QueryMode = 'sql' | 'promql';

/** The shape the backend turns the response into. */
export type ResultFormat = 'auto' | 'logs' | 'table' | 'timeseries' | 'trace' | 'annotations';

/** Editor mode, kept for compatibility with old saved dashboards. */
export type EditorMode = 'code' | 'builder';

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
  /** Legacy field kept so old dashboards keep loading. */
  editorMode?: EditorMode;

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
  enableSSE: true,
  useCache: true,
};

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
