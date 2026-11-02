import { DataSourceInstanceSettings } from '@grafana/data';

import { OpenObserveDataSource } from './datasource';
import { OpenObserveOptions } from './types';

// The datasource only talks to Grafana through DataSourceWithBackend, so the
// tests mock the resource/query plumbing instead of hitting a live backend.
jest.mock('@grafana/runtime', () => {
  const actual = jest.requireActual('@grafana/runtime');
  return {
    ...actual,
    getTemplateSrv: () => ({
      replace: (value: string) => value.replace(/\$var/g, 'vnet-bff'),
    }),
  };
});

function newDatasource(jsonData: Partial<OpenObserveOptions> = {}): OpenObserveDataSource {
  const settings = {
    id: 1,
    uid: 'oo',
    type: 'grafana-openobserve-datasource',
    name: 'OpenObserve',
    url: 'http://localhost:5080',
    jsonData,
    access: 'proxy',
    meta: {},
    readOnly: false,
    withCredentials: false,
  } as unknown as DataSourceInstanceSettings<OpenObserveOptions>;
  return new OpenObserveDataSource(settings);
}

describe('OpenObserveDataSource', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('falls back to the legacy "database" key for the organization', () => {
    expect(newDatasource({ database: 'legacy-org' }).organization).toBe('legacy-org');
    expect(newDatasource({ organization: 'new-org' }).organization).toBe('new-org');
    expect(newDatasource({}).organization).toBe('default');
  });

  it('prefers organization over the legacy database key', () => {
    expect(newDatasource({ organization: 'new-org', database: 'legacy-org' }).organization).toBe('new-org');
  });

  it('treats a query with neither SQL nor PromQL as empty', () => {
    const ds = newDatasource();
    expect(ds.filterQuery({ refId: 'A', queryType: 'logs' })).toBe(false);
    expect(ds.filterQuery({ refId: 'A', queryType: 'logs', rawSql: 'SELECT 1' })).toBe(true);
    expect(ds.filterQuery({ refId: 'A', queryType: 'metrics', expr: 'up' })).toBe(true);
  });

  it('keeps trace queries even though the backend writes their SQL', () => {
    // Trace queries describe themselves with structured fields; filtering them
    // out here meant they were never sent, so the panel always showed no data.
    const ds = newDatasource();
    expect(ds.filterQuery({ refId: 'A', queryType: 'traces', table: 'prd' })).toBe(true);
    expect(ds.filterQuery({ refId: 'A', queryType: 'traces', traceMode: 'search' })).toBe(true);
    expect(ds.filterQuery({ refId: 'A', queryType: 'traces', traceId: 'abc' })).toBe(true);
    // A trace query with nothing configured still has nothing to run.
    expect(ds.filterQuery({ refId: 'A', queryType: 'traces' })).toBe(false);
  });

  it('applies template variables to SQL and PromQL', () => {
    const ds = newDatasource();
    const updated = ds.applyTemplateVariables(
      { refId: 'A', queryType: 'logs', rawSql: 'SELECT * FROM "logs" WHERE svc = $var', expr: 'up{svc="$var"}' },
      {}
    );
    expect(updated.rawSql).toBe('SELECT * FROM "logs" WHERE svc = vnet-bff');
    expect(updated.expr).toBe('up{svc="vnet-bff"}');
  });

  it('keeps the ad-hoc filters the dashboard passed in', () => {
    const ds = newDatasource();
    const filters = [{ key: 'service_name', operator: '=', value: 'svc-a' }];
    const updated = ds.applyTemplateVariables({ refId: 'A', queryType: 'logs', rawSql: 'SELECT 1' }, {}, filters);
    expect(updated.adhocFilters).toEqual(filters);
  });

  it('keeps the ad-hoc filters stored on the query when none are passed', () => {
    const ds = newDatasource();
    const filters = [{ key: 'service_name', operator: '=', value: 'svc-a' }];
    const updated = ds.applyTemplateVariables(
      { refId: 'A', queryType: 'logs', rawSql: 'SELECT 1', adhocFilters: filters },
      {}
    );
    expect(updated.adhocFilters).toEqual(filters);
  });

  it('forwards filters through interpolateVariablesInQueries', () => {
    const ds = newDatasource();
    const filters = [{ key: 'level', operator: '=', value: 'error' }];
    const [query] = ds.interpolateVariablesInQueries([{ refId: 'A', queryType: 'logs', rawSql: 'SELECT $var' }], {}, filters);
    expect(query.rawSql).toBe('SELECT vnet-bff');
    expect(query.adhocFilters).toEqual(filters);
  });

  it('caches the stream list per stream type', async () => {
    const ds = newDatasource();
    const spy = jest
      .spyOn(ds, 'getResource')
      .mockResolvedValue({ app: ['_timestamp', 'body'], other: ['x'] } as never);

    const first = await ds.listStreams('logs');
    const second = await ds.listStreams('logs');
    expect(first).toEqual(['app', 'other']);
    expect(second).toEqual(['app', 'other']);
    expect(spy).toHaveBeenCalledTimes(1);

    // A different stream type is a different cache entry.
    await ds.listStreams('metrics');
    expect(spy).toHaveBeenCalledTimes(2);
  });

  it('drops a failed stream request so the next call retries', async () => {
    const ds = newDatasource();
    const spy = jest
      .spyOn(ds, 'getResource')
      .mockRejectedValueOnce(new Error('boom') as never)
      .mockResolvedValueOnce({ app: ['body'] } as never);

    await expect(ds.listStreams('logs')).rejects.toThrow('boom');
    await expect(ds.listStreams('logs')).resolves.toEqual(['app']);
    expect(spy).toHaveBeenCalledTimes(2);
  });

  it('returns no fields when the stream has no cached columns', async () => {
    const ds = newDatasource();
    jest.spyOn(ds, 'getResource').mockResolvedValue({} as never);
    await expect(ds.fetchFields('logs', undefined)).resolves.toEqual([]);
    await expect(ds.fetchFields('logs', 'missing')).resolves.toEqual([]);
  });

  it('maps the stream columns to completion entries', async () => {
    const ds = newDatasource();
    jest.spyOn(ds, 'getResource').mockResolvedValue({ app: ['_timestamp', 'body'] } as never);
    await expect(ds.fetchFields('logs', 'app')).resolves.toEqual([
      { name: '_timestamp', type: 'string', description: '_timestamp' },
      { name: 'body', type: 'string', description: 'body' },
    ]);
  });

  it('asks the backend for tag keys with the selected stream', async () => {
    const ds = newDatasource({ organization: 'org-1' });
    const spy = jest.spyOn(ds, 'getResource').mockResolvedValue(['service_name'] as never);

    const response = await ds.getTagKeys({ queries: [{ refId: 'A', queryType: 'logs', table: 'app' }] } as never);
    expect(response.data).toEqual([{ text: 'service_name' }]);
    expect(spy).toHaveBeenCalledWith('/openobserve/tag-keys', {
      organization: 'org-1',
      type: 'logs',
      stream: 'app',
    });
  });

  it('derives the stream from the SQL when the query has no table set', async () => {
    // A hand-written SQL leaves `table` empty; the ad-hoc filter still has to
    // receive column names, so the stream is read out of the statement.
    const ds = newDatasource();
    const spy = jest.spyOn(ds, 'getResource').mockResolvedValue(['body', 'severity'] as never);

    const response = await ds.getTagKeys({
      queries: [{ refId: 'A', queryType: 'logs', rawSql: 'SELECT * FROM "vnet_bff_dev" LIMIT 10' }],
    } as never);

    expect(response.data).toEqual([{ text: 'body' }, { text: 'severity' }]);
    expect(spy).toHaveBeenCalledWith('/openobserve/tag-keys', {
      organization: 'default',
      type: 'logs',
      stream: 'vnet_bff_dev',
    });
  });

  it('prefers the explicit table over the SQL stream', async () => {
    const ds = newDatasource();
    const spy = jest.spyOn(ds, 'getResource').mockResolvedValue([] as never);

    await ds.getTagKeys({
      queries: [{ refId: 'A', queryType: 'logs', table: 'picked', rawSql: 'SELECT * FROM "other"' }],
    } as never);

    expect(spy).toHaveBeenCalledWith('/openobserve/tag-keys', {
      organization: 'default',
      type: 'logs',
      stream: 'picked',
    });
  });

  it('derives the stream from the SQL for tag values too', async () => {
    const ds = newDatasource();
    const spy = jest.spyOn(ds, 'getResource').mockResolvedValue(['INFO'] as never);

    await ds.getTagValues({
      key: 'severity',
      queries: [{ refId: 'A', queryType: 'logs', rawSql: 'SELECT * FROM `vnet_bff_dev`' }],
    } as never);

    expect(spy).toHaveBeenCalledWith('/openobserve/tag-values', {
      organization: 'default',
      type: 'logs',
      stream: 'vnet_bff_dev',
      key: 'severity',
    });
  });

  it('omits the stream for tag keys when none is selected', async () => {
    const ds = newDatasource();
    const spy = jest.spyOn(ds, 'getResource').mockResolvedValue([] as never);

    await ds.getTagKeys({ queries: [{ refId: 'A', queryType: 'metrics' }] } as never);
    expect(spy).toHaveBeenCalledWith('/openobserve/tag-keys', {
      organization: 'default',
      type: 'metrics',
    });
  });

  it('passes the time range to tag values in microseconds', async () => {
    const ds = newDatasource();
    const spy = jest.spyOn(ds, 'getResource').mockResolvedValue(['svc-a'] as never);

    const response = await ds.getTagValues({
      key: 'service_name',
      queries: [{ refId: 'A', queryType: 'logs', table: 'app' }],
      timeRange: { from: { valueOf: () => 1000 }, to: { valueOf: () => 2000 } },
    } as never);

    expect(response.data).toEqual([{ text: 'svc-a' }]);
    expect(spy).toHaveBeenCalledWith('/openobserve/tag-values', {
      organization: 'default',
      type: 'logs',
      stream: 'app',
      key: 'service_name',
      timeFrom: '1000000',
      timeTo: '2000000',
    });
  });
});
