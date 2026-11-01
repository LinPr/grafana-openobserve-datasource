import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { TraceQueryEditor } from './TraceQueryEditor';
import { OpenObserveDataSource } from '../datasource';
import { OpenObserveQuery } from '../types';

jest.mock('@grafana/ui', () => {
  const actual = jest.requireActual('@grafana/ui');
  return {
    ...actual,
    RadioButtonGroup: ({ options, value, onChange }: any) => (
      <div role="group" aria-label="trace-mode">
        {options.map((o: any) => (
          <button key={o.value} type="button" aria-pressed={o.value === value} onClick={() => onChange(o.value)}>
            {o.label}
          </button>
        ))}
      </div>
    ),
    Select: ({ inputId, value, options, onChange, placeholder }: any) => (
      <select
        aria-label={inputId ?? placeholder}
        data-testid={inputId}
        value={value ?? ''}
        onChange={e => {
          const o = (options ?? []).find((x: any) => String(x.value) === e.currentTarget.value);
          if (o) {
            onChange(o);
          }
        }}
      >
        <option value="">{placeholder ?? ''}</option>
        {(options ?? []).map((o: any) => (
          <option key={String(o.value)} value={String(o.value)}>
            {o.label}
          </option>
        ))}
      </select>
    ),
  };
});

function newDatasource(overrides: Record<string, unknown> = {}) {
  return {
    listStreams: jest.fn().mockResolvedValue(['prd', 'dev']),
    distinctTraceValues: jest.fn().mockImplementation((_stream: string, key: string) =>
      Promise.resolve(key === 'service_name' ? ['vnet-bff', 'gateway'] : ['HTTP GET', 'GET /healthz'])
    ),
    ...overrides,
  } as unknown as OpenObserveDataSource;
}

function renderEditor(query: Partial<OpenObserveQuery> = {}, datasource = newDatasource()) {
  const onChange = jest.fn();
  const onRunQuery = jest.fn();
  const full = { refId: 'A', queryType: 'traces', table: 'prd', ...query } as OpenObserveQuery;

  const view = render(
    <TraceQueryEditor datasource={datasource} query={full} onChange={onChange} onRunQuery={onRunQuery} />
  );
  const input = (id: string) => view.container.querySelector<HTMLInputElement>(`#${id}`)!;
  return { onChange, onRunQuery, datasource, query: full, input, view };
}

const lastQuery = (fn: jest.Mock): OpenObserveQuery => fn.mock.calls[fn.mock.calls.length - 1][0];

describe('TraceQueryEditor', () => {
  afterEach(() => jest.clearAllMocks());

  it('offers the three Jaeger-style query types', () => {
    renderEditor();
    for (const label of ['Search', 'TraceID', 'Graph']) {
      expect(screen.getByRole('button', { name: label })).toBeInTheDocument();
    }
  });

  it('shows the search filters in Search mode', () => {
    const { input } = renderEditor({ traceMode: 'search' });
    expect(screen.getByTestId('trace-editor-service')).toBeInTheDocument();
    expect(screen.getByTestId('trace-editor-operation')).toBeInTheDocument();
    expect(input('trace-editor-tags')).toBeInTheDocument();
    expect(input('trace-editor-min-duration')).toBeInTheDocument();
    expect(input('trace-editor-limit')).toBeInTheDocument();
    // The trace-id input belongs to the other mode.
    expect(input('trace-editor-trace-id')).not.toBeInTheDocument();
  });

  it('shows only the trace id field in TraceID mode', () => {
    const { input } = renderEditor({ traceMode: 'traceId' });
    expect(input('trace-editor-trace-id')).toBeInTheDocument();
    expect(screen.queryByTestId('trace-editor-service')).not.toBeInTheDocument();
    expect(input('trace-editor-tags')).not.toBeInTheDocument();
  });

  it('switches mode and runs the query', async () => {
    const { onChange, onRunQuery } = renderEditor({ traceMode: 'search' });
    fireEvent.click(screen.getByRole('button', { name: 'TraceID' }));
    expect(lastQuery(onChange).traceMode).toBe('traceId');
    expect(onRunQuery).toHaveBeenCalled();
  });

  it('suggests services and operations from the stream', async () => {
    const datasource = newDatasource();
    renderEditor({ traceMode: 'search' }, datasource);

    await waitFor(() => expect(datasource.distinctTraceValues).toHaveBeenCalledWith('prd', 'service_name'));
    await waitFor(() => {
      const opts = Array.from(screen.getByTestId('trace-editor-service').querySelectorAll('option')).map(
        o => o.textContent
      );
      expect(opts).toContain('vnet-bff');
    });
  });

  it('splits the tag input on whitespace', () => {
    const { onChange, input } = renderEditor({ traceMode: 'search' });
    fireEvent.change(input('trace-editor-tags'), {
      target: { value: 'error=true  http.status_code=200 ' },
    });
    expect(lastQuery(onChange).traceTags).toEqual(['error=true', 'http.status_code=200']);
  });

  it('keeps duration strings verbatim for the backend to parse', () => {
    const { onChange, input } = renderEditor({ traceMode: 'search' });
    fireEvent.change(input('trace-editor-min-duration'), { target: { value: '100ms' } });
    expect(lastQuery(onChange).minDuration).toBe('100ms');
  });

  it('runs the query when Enter is pressed in the trace id field', () => {
    const { onRunQuery, input } = renderEditor({ traceMode: 'traceId', traceId: '' });
    fireEvent.keyDown(input('trace-editor-trace-id'), { key: 'Enter' });
    expect(onRunQuery).toHaveBeenCalled();
  });

  it('explains the graph output', () => {
    renderEditor({ traceMode: 'graph' });
    expect(screen.getByText(/caller → callee/i)).toBeInTheDocument();
    expect(screen.getByTestId('trace-editor-graph-service')).toBeInTheDocument();
  });

  it('reports a stream listing failure instead of rendering an empty picker silently', async () => {
    const datasource = newDatasource({
      listStreams: jest.fn().mockRejectedValue({ data: { error: 'Organization not found' } }),
    });
    renderEditor({}, datasource);
    expect(await screen.findByRole('alert')).toHaveTextContent('Organization not found');
  });
});
