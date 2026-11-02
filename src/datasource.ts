import {
  AdHocVariableFilter,
  CoreApp,
  DataSourceGetTagKeysOptions,
  DataSourceGetTagValuesOptions,
  DataSourceInstanceSettings,
  GetTagResponse,
  ScopedVars,
  dateTime,
} from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';
import {
  DEFAULT_QUERY,
  OpenObserveOptions,
  OpenObserveQuery,
  SchemaField,
  StreamColumns,
  StreamType,
} from './types';
import { extractFirstTable } from './utils/completion';
import { replace } from './utils/variables';
import { OpenObserveVariableSupport } from './variables';

/**
 * Grafana data source for OpenObserve.
 *
 * All heavy lifting (SQL parsing, HTTP/SSE, frame building) happens in the Go
 * backend; this class is the thin frontend facade: variable interpolation,
 * tag keys/values for ad-hoc filters, template variable support and the small
 * resource calls the query editor needs for completion.
 */
export class OpenObserveDataSource extends DataSourceWithBackend<OpenObserveQuery, OpenObserveOptions> {
  /**
   * Annotation support. Grafana runs the annotation target through the normal
   * query path and maps the resulting frame onto events, so forcing the
   * "annotations" format is all that is needed.
   */
  annotations = {
    getDefaultQuery: (): Partial<OpenObserveQuery> => ({
      ...DEFAULT_QUERY,
      format: 'annotations',
    }),
    prepareQuery: (annotation: { target?: Partial<OpenObserveQuery>; query?: string }): OpenObserveQuery | undefined => {
      const target: Partial<OpenObserveQuery> = annotation.target ?? {};
      // Older dashboards stored the SQL in a plain "query" string.
      const rawSql = target.rawSql ?? annotation.query ?? '';
      if (!rawSql && !target.expr) {
        return undefined;
      }
      return { ...DEFAULT_QUERY, ...target, rawSql, format: 'annotations' } as OpenObserveQuery;
    },
  };

  /** The organization configured on the instance. */
  readonly organization: string;

  /** Per-stream-type completion cache: the editor asks for it on every keypress. */
  private streamCache = new Map<string, Promise<StreamColumns>>();

  constructor(private instanceSettings: DataSourceInstanceSettings<OpenObserveOptions>) {
    super(instanceSettings);
    this.organization = this.instanceSettings.jsonData?.organization || this.instanceSettings.jsonData?.database || 'default';
    this.variables = new OpenObserveVariableSupport(this);
  }

  getDefaultQuery(_app: CoreApp): Partial<OpenObserveQuery> {
    return DEFAULT_QUERY;
  }

  /**
   * Applies template variables and ad-hoc filters to a query before it is sent
   * to the backend.
   */
  applyTemplateVariables(query: OpenObserveQuery, scopedVars: ScopedVars, filters?: AdHocVariableFilter[]): OpenObserveQuery {
    const replaced = replace(query.rawSql ?? '', scopedVars) ?? '';
    const expr = replace(query.expr ?? '', scopedVars) ?? '';
    return {
      ...query,
      rawSql: replaced,
      expr,
      // Keep the ad-hoc filters the dashboard passed in; fall back to the ones
      // stored on the query for Explore.
      adhocFilters: filters ?? query.adhocFilters ?? [],
    };
  }

  /** Template interpolation for variables embedded in the PromQL expression. */
  interpolateVariablesInQueries(
    queries: OpenObserveQuery[],
    scopedVars: ScopedVars,
    filters?: AdHocVariableFilter[]
  ): OpenObserveQuery[] {
    if (!queries.length) {
      return queries;
    }
    return queries.map(q => this.applyTemplateVariables(q, scopedVars, filters));
  }

  filterQuery(query: OpenObserveQuery): boolean {
    // Trace queries carry their parameters in the structured fields and let the
    // backend build the SQL, so they have no rawSql/expr. Filtering them out
    // here dropped every trace query before it was ever sent.
    if (query.queryType === 'traces') {
      return Boolean(query.table || query.traceId || query.traceMode);
    }
    return Boolean(query.rawSql || query.expr);
  }

  /**
   * Ad-hoc filter keys: the columns of the stream the query reads.
   *
   * `query.table` is only set when the stream was picked from the dropdown; a
   * hand-written SQL usually leaves it empty, so the stream is read out of the
   * statement. Without this the backend would answer with the *stream names*
   * instead of the column names, which makes the ad-hoc filter unusable.
   */
  async getTagKeys(options?: DataSourceGetTagKeysOptions<OpenObserveQuery>): Promise<GetTagResponse> {
    const query = options?.queries?.[0];
    const streamType = query?.queryType ?? 'logs';
    const params: Record<string, string> = {
      organization: this.organization,
      type: streamType,
    };
    const stream = query?.table ?? extractFirstTable(query?.rawSql ?? '');
    if (stream) {
      params.stream = stream;
    }
    const keys: string[] = await this.getResource('/openobserve/tag-keys', params);
    return { data: keys.map(text => ({ text })) } as GetTagResponse;
  }

  /** Ad-hoc filter values: the distinct values of one column. */
  async getTagValues(options: DataSourceGetTagValuesOptions<OpenObserveQuery>): Promise<GetTagResponse> {
    const query = options?.queries?.[0];
    const streamType = query?.queryType ?? 'logs';
    const params: Record<string, string> = {
      organization: this.organization,
      type: streamType,
      // Same fallback as getTagKeys: the stream may only exist in the SQL.
      stream: query?.table ?? extractFirstTable(query?.rawSql ?? '') ?? '',
      key: options.key,
    };
    if (options.timeRange) {
      params.timeFrom = String(options.timeRange.from.valueOf() * 1000);
      params.timeTo = String(options.timeRange.to.valueOf() * 1000);
    }
    const values: string[] = await this.getResource('/openobserve/tag-values', params);
    return { data: values.map(text => ({ text })) } as GetTagResponse;
  }

  /** Metric names for PromQL completion and metric-aware variables. */
  async listMetricNames(): Promise<string[]> {
    return await this.getResource('/openobserve/metric-names', { organization: this.organization });
  }

  /**
   * listStreams returns the stream names of one type.
   */
  async listStreams(streamType: StreamType | string = 'logs'): Promise<string[]> {
    const columns = await this.listStreamColumns(streamType);
    return Object.keys(columns).sort();
  }

  /**
   * listStreamColumns returns stream -> columns, cached per stream type so the
   * SQL editor's completion does not hammer the backend.
   */
  async listStreamColumns(streamType: StreamType | string = 'logs'): Promise<StreamColumns> {
    const key = streamType || 'logs';
    const cached = this.streamCache.get(key);
    if (cached) {
      return cached;
    }
    const request = this.getResource<StreamColumns>('/openobserve/streams', {
      organization: this.organization,
      type: key,
      withSchema: 'true',
    })
      .then(columns => {
        if (!columns || typeof columns !== 'object') {
          return {};
        }
        return columns;
      })
      .catch(error => {
        // Drop the failed promise so the next keystroke retries.
        this.streamCache.delete(key);
        throw error;
      });
    this.streamCache.set(key, request);
    return request;
  }

  /** fetchFields returns the completion entries for one stream. */
  async fetchFields(streamType: StreamType | string, table?: string): Promise<Array<{ name: string; type: string; description: string }>> {
    if (!table) {
      return [];
    }
    const columns = await this.listStreamColumns(streamType);
    const fields = columns[table];
    if (!fields) {
      return [];
    }
    return fields.map(name => ({ name, type: 'string', description: name }));
  }

  /** The typed schema of one stream, used by the query editor. */
  async fetchSchema(streamType: StreamType | string, stream: string): Promise<SchemaField[]> {
    return await this.getResource<SchemaField[]>('/openobserve/schema', {
      organization: this.organization,
      type: streamType,
      stream,
    });
  }

  /**
   * Distinct values of one trace column, used to suggest services and
   * operations in the trace editor. Failures degrade to an empty list: these are
   * suggestions, not required data.
   */
  async distinctTraceValues(stream: string, key: string): Promise<string[]> {
    if (!stream || !key) {
      return [];
    }
    try {
      const values: string[] = await this.getResource('/openobserve/tag-values', {
        organization: this.organization,
        type: 'traces',
        stream,
        key,
      });
      return Array.isArray(values) ? values : [];
    } catch {
      return [];
    }
  }

  /**
   * Reports whether a stream has any row in the query's time range.
   *
   * Used to pick a useful default stream: catalogues include stale streams, and
   * defaulting to an empty one makes a freshly switched query look broken.
   */
  async streamHasData(streamType: StreamType | string, stream: string, range?: { from: number; to: number }): Promise<boolean> {
    const now = Date.now();
    const to = range && range.to > 0 ? range.to : now;
    const from = range && range.from > 0 ? range.from : to - 15 * 60 * 1000;
    const probe: OpenObserveQuery = {
      refId: 'probe',
      queryType: streamType as OpenObserveQuery['queryType'],
      queryMode: 'sql',
      format: 'table',
      // A one-row LIMIT is enough: the question is only "does anything exist?".
      rawSql: `SELECT 1 AS present FROM "${stream.replace(/"/g, '""')}" LIMIT 1`,
      size: 1,
    };
    try {
      const response = await this.query({
        targets: [probe],
        range: { from: dateTime(from), to: dateTime(to), raw: { from: `now-15m`, to: 'now' } },
        interval: '1m',
        intervalMs: 60000,
        maxDataPoints: 1,
        scopedVars: {},
        requestId: `probe-${stream}`,
        timezone: 'utc',
        app: CoreApp.Explore,
        startTime: 0,
      } as never);
      const first = await new Promise<unknown>((resolve) => {
        const sub = (response as { subscribe: (o: unknown) => { unsubscribe: () => void } }).subscribe({
          next: (frame: { data?: Array<{ length?: number }> }) => {
            resolve(frame?.data?.[0]?.length ?? 0);
            sub.unsubscribe();
          },
          error: () => resolve(0),
        });
      });
      return Number(first) > 0;
    } catch {
      return false;
    }
  }

  /** Invalidate the completion cache, e.g. after creating a stream. */
  clearCache(): void {
    this.streamCache.clear();
  }

  /** Exposes the datasource for the variable support's query method. */
  get instance(): DataSourceInstanceSettings<OpenObserveOptions> {
    return this.instanceSettings;
  }
}
