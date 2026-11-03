import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { CoreApp } from '@grafana/data';

import { QueryEditorSQL } from './QueryEditor';
import { OpenObserveDataSource } from '../datasource';
import { OpenObserveQuery } from '../types';

// @grafana/ui Select renders a real menu; drive it through a plain <select> so
// the assertions stay about the editor's behaviour, not about the widget.
jest.mock('@grafana/ui', () => {
  const actual = jest.requireActual('@grafana/ui');
  return {
    ...actual,
    CodeEditor: ({ value, language, onChange, onBlur }: any) => (
      <textarea
        aria-label={`code-${language}`}
        data-language={language}
        value={value}
        onChange={event => onChange(event.currentTarget.value)}
        onBlur={event => onBlur(event.currentTarget.value)}
      />
    ),
    RadioButtonGroup: ({ options, value, onChange, 'aria-label': ariaLabel }: any) => (
      <div role="group" aria-label={ariaLabel ?? 'stream-type'}>
        {(options ?? []).map((option: any) => (
          <button
            key={String(option.value)}
            type="button"
            aria-pressed={option.value === value}
            onClick={() => onChange(option.value)}
          >
            {option.label}
          </button>
        ))}
      </div>
    ),
    Select: ({ inputId, value, options, onChange, disabled, placeholder }: any) => (
      <select
        aria-label={inputId ?? placeholder ?? 'select'}
        data-testid={inputId}
        value={value ?? ''}
        disabled={disabled}
        onChange={event => {
          const option = (options ?? []).find((o: any) => String(o.value) === event.currentTarget.value);
          if (option) {
            onChange(option);
          }
        }}
      >
        <option value="">{placeholder ?? ''}</option>
        {(options ?? []).map((option: any) => (
          <option key={String(option.value)} value={String(option.value)}>
            {option.label}
          </option>
        ))}
      </select>
    ),
  };
});

function newDatasource(overrides: Record<string, unknown> = {}) {
  const ds = {
    listStreams: jest.fn().mockResolvedValue(['vnet_bff_dev', 'alerta']),
    fetchSchema: jest.fn().mockResolvedValue([]),
    listStreamColumns: jest.fn().mockResolvedValue({ vnet_bff_dev: ['body', 'severity'], alerta: ['body'] }),
    listMetricNames: jest.fn().mockResolvedValue([]),
    distinctTraceValues: jest.fn().mockResolvedValue([]),
    streamHasData: jest.fn().mockResolvedValue(true),
    ...overrides,
  } as unknown as OpenObserveDataSource;
  return ds;
}

type RenderOptions = {
  query?: Partial<OpenObserveQuery>;
  datasource?: OpenObserveDataSource;
};

/**
 * The editor is a controlled component: it shows whatever `query` the parent
 * passes back. Wrapping it in a state holder is what makes "change a control,
 * then observe the next render" testable.
 */
function Harness({
  initial,
  datasource,
  onChange,
  onRunQuery,
}: {
  initial: OpenObserveQuery;
  datasource: OpenObserveDataSource;
  onChange: jest.Mock;
  onRunQuery: jest.Mock;
}) {
  const [query, setQuery] = React.useState<OpenObserveQuery>(initial);
  return (
    <QueryEditorSQL
      datasource={datasource}
      query={query}
      onChange={next => {
        onChange(next);
        setQuery(next as OpenObserveQuery);
      }}
      onRunQuery={onRunQuery}
      onBlur={() => undefined}
      data={undefined}
      range={undefined as never}
      app={CoreApp.Explore}
      queries={[query]}
      {...({} as Record<string, unknown>)}
    />
  );
}

function renderEditor(options: RenderOptions = {}) {
  const onChange = jest.fn();
  const onRunQuery = jest.fn();
  // Default these fixtures to Code: the suites below drive the SQL editor.
  const query = { refId: 'A', queryType: 'logs', editorMode: 'code', ...options.query } as OpenObserveQuery;
  const datasource = options.datasource ?? newDatasource();

  const view = render(
    <Harness initial={query} datasource={datasource} onChange={onChange} onRunQuery={onRunQuery} />
  );

  return { onChange, onRunQuery, datasource, query, view };
}

const lastChange = (onChange: jest.Mock): OpenObserveQuery => onChange.mock.calls[onChange.mock.calls.length - 1][0];

describe('QueryEditorSQL', () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  it('renders the SQL editor for a SQL query', () => {
    renderEditor({ query: { queryMode: 'sql', rawSql: 'SELECT * FROM "vnet_bff_dev" LIMIT 10' } });
    expect(screen.getByLabelText('code-sql')).toHaveValue('SELECT * FROM "vnet_bff_dev" LIMIT 10');
    expect(screen.queryByLabelText('code-promql')).not.toBeInTheDocument();
  });

  it('renders the PromQL editor for a PromQL query', () => {
    renderEditor({ query: { queryType: 'metrics', queryMode: 'promql', expr: 'go_goroutine_count' } });
    expect(screen.getByLabelText('code-promql')).toHaveValue('go_goroutine_count');
    expect(screen.queryByLabelText('code-sql')).not.toBeInTheDocument();
  });

  it('disables PromQL unless the stream type is metrics', async () => {
    renderEditor({ query: { queryType: 'logs' } });
    // PromQL has no meaning outside metric streams, so the picker is disabled.
    await waitFor(() => expect(screen.getByTestId('query-editor-mode')).toBeDisabled());
  });

  it('enables PromQL for metrics streams', async () => {
    renderEditor({ query: { queryType: 'metrics' } });
    await waitFor(() => expect(screen.getByTestId('query-editor-mode')).not.toBeDisabled());
  });

  it('resets the query mode to SQL when leaving the metrics stream type', async () => {
    const { onChange } = renderEditor({ query: { queryType: 'metrics', queryMode: 'promql', expr: 'up' } });
    await waitFor(() => expect(screen.getByRole('group', { name: 'stream-type' })).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Logs' }));

    const updated = lastChange(onChange);
    expect(updated.queryType).toBe('logs');
    expect(updated.queryMode).toBe('sql');
  });

  it('lists the streams of the selected type', async () => {
    const datasource = newDatasource();
    renderEditor({ datasource, query: { queryType: 'logs' } });

    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('logs'));
    await waitFor(() => {
      const options = Array.from(screen.getByTestId('query-editor-stream').querySelectorAll('option')).map(
        o => o.textContent
      );
      expect(options).toContain('vnet_bff_dev');
      expect(options).toContain('alerta');
    });
  });

  it('reloads the stream list when the stream type changes', async () => {
    const datasource = newDatasource();
    renderEditor({ datasource, query: { queryType: 'logs' } });
    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('logs'));

    fireEvent.click(screen.getByRole('button', { name: 'Traces' }));
    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('traces'));
  });

  it('shows a warning instead of failing when streams cannot be listed', async () => {
    const datasource = newDatasource({
      listStreams: jest.fn().mockRejectedValue({ data: { error: 'Organization not found' } }),
    });
    renderEditor({ datasource });

    expect(await screen.findByRole('alert')).toHaveTextContent('Organization not found');
  });

  it('rewrites the FROM clause when a stream is picked', async () => {
    const { onChange, onRunQuery } = renderEditor({
      query: { queryType: 'logs', rawSql: 'SELECT * FROM "alerta" LIMIT 100' },
    });
    await waitFor(() =>
      expect(screen.getByTestId('query-editor-stream').querySelectorAll('option').length).toBeGreaterThan(1)
    );

    fireEvent.change(screen.getByTestId('query-editor-stream'), { target: { value: 'vnet_bff_dev' } });

    const updated = lastChange(onChange);
    expect(updated.table).toBe('vnet_bff_dev');
    expect(updated.rawSql).toBe('SELECT * FROM "vnet_bff_dev" LIMIT 100');
    expect(onRunQuery).toHaveBeenCalled();
  });

  it('seeds a SELECT when the query has no FROM clause yet', async () => {
    const { onChange } = renderEditor({ query: { queryType: 'logs', rawSql: '' } });
    await waitFor(() =>
      expect(screen.getByTestId('query-editor-stream').querySelectorAll('option').length).toBeGreaterThan(1)
    );

    fireEvent.change(screen.getByTestId('query-editor-stream'), { target: { value: 'alerta' } });

    expect(lastChange(onChange).rawSql).toBe('SELECT * FROM "alerta" LIMIT 100');
  });

  it('tracks the stream when the SQL is edited by hand', async () => {
    const { onChange } = renderEditor({ query: { queryType: 'logs', rawSql: 'SELECT 1' } });
    const editor = screen.getByLabelText('code-sql');

    fireEvent.change(editor, { target: { value: 'SELECT * FROM "alerta" LIMIT 5' } });

    await waitFor(() => expect(lastChange(onChange).table).toBe('alerta'));
  });

  it('no longer offers a Format picker', () => {
    // The backend infers the frame shape from the stream type and the query, so
    // a separate Format switch could only contradict it.
    renderEditor({ query: { queryType: 'logs' } });
    expect(screen.queryByTestId('query-editor-format')).not.toBeInTheDocument();
  });

  it('mirrors the Limit field into the SQL and back', async () => {
    const { onChange, view } = renderEditor({
      query: { queryType: 'logs', rawSql: 'SELECT * FROM "x" LIMIT 10', size: 10 },
    });
    const limitInput = view.container.querySelector<HTMLInputElement>('#query-editor-limit')!;

    // Typing a limit rewrites the statement's LIMIT.
    fireEvent.change(limitInput, { target: { value: '42' } });
    let updated = lastChange(onChange);
    expect(updated.size).toBe(42);
    expect(updated.rawSql).toBe('SELECT * FROM "x" LIMIT 42');

    // ... and a LIMIT typed into the editor updates the field.
    fireEvent.change(screen.getByLabelText('code-sql'), {
      target: { value: 'SELECT * FROM "x" LIMIT 7' },
    });
    updated = lastChange(onChange);
    expect(updated.size).toBe(7);
  });

  it('sends the limit as a number', async () => {
    const { onChange } = renderEditor({ query: { size: 0 } });
    const input = screen.getByLabelText('Limit', { selector: 'input' });

    fireEvent.change(input, { target: { value: '250' } });

    await waitFor(() => expect(lastChange(onChange).size).toBe(250));
  });

  it('toggles SSE and keeps the query runnable', async () => {
    const { onChange, onRunQuery, view } = renderEditor({ query: { enableSSE: true } });

    const input = view.container.querySelector<HTMLInputElement>('#query-editor-sse');
    expect(input).not.toBeNull();
    fireEvent.click(input!);

    expect(lastChange(onChange).enableSSE).toBe(false);
    expect(onRunQuery).toHaveBeenCalled();
  });

  it('toggles the result cache', async () => {
    const { onChange, view } = renderEditor({ query: { useCache: true } });

    const input = view.container.querySelector<HTMLInputElement>('#query-editor-cache');
    expect(input).not.toBeNull();
    fireEvent.click(input!);

    expect(lastChange(onChange).useCache).toBe(false);
  });

  it('shows a hint when the SQL is still empty', () => {
    renderEditor({ query: { queryType: 'logs', queryMode: 'sql', rawSql: '' } });
    expect(screen.getByText(/SELECT \* FROM/i)).toBeInTheDocument();
  });
});

describe('QueryEditorSQL / builder and code isolation', () => {
  afterEach(() => jest.clearAllMocks());

  it('defaults a query without a statement to the builder', () => {
    renderEditor({ query: { queryType: 'logs', editorMode: undefined, rawSql: '' } });
    expect(screen.getByTestId('query-builder')).toBeInTheDocument();
    // The SQL editor must not be on screen: picking options used to render
    // generated SQL into it, which is what this split exists to prevent.
    expect(screen.queryByLabelText('code-sql')).not.toBeInTheDocument();
  });

  it('opens an existing statement in code mode', () => {
    // A saved query with SQL was written by hand; hiding it in the builder
    // would look like data loss.
    renderEditor({ query: { queryType: 'logs', editorMode: undefined, rawSql: 'SELECT * FROM "x"' } });
    expect(screen.getByLabelText('code-sql')).toBeInTheDocument();
    expect(screen.queryByTestId('query-builder')).not.toBeInTheDocument();
  });

  it('never writes generated SQL into the statement', async () => {
    const datasource = newDatasource({
      listStreams: jest.fn().mockResolvedValue(['vnet_bff_dev', 'alerta']),
    });
    const { onChange } = renderEditor({
      datasource,
      query: { queryType: 'logs', editorMode: 'builder', rawSql: 'SELECT * FROM "mine" WHERE a = 1', builderStream: '' },
    });
    // The stream list is fetched; the option has to exist before it can be picked.
    await waitFor(() => expect(screen.getByTestId('builder-stream').querySelectorAll('option').length).toBeGreaterThan(1));

    fireEvent.change(screen.getByTestId('builder-stream'), { target: { value: 'vnet_bff_dev' } });

    const updated = lastChange(onChange);
    expect(updated.builderStream).toBe('vnet_bff_dev');
    // The hand-written statement survives untouched.
    expect(updated.rawSql).toBe('SELECT * FROM "mine" WHERE a = 1');
  });

  it('keeps builder fields when switching to code and back', () => {
    const { onChange } = renderEditor({
      query: { queryType: 'logs', editorMode: 'builder', rawSql: '', builderStream: 'vnet_bff_dev', builderAggregation: 'avg' },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Code' }));
    const toCode = lastChange(onChange);
    expect(toCode.editorMode).toBe('code');
    // Switching modes is a flag flip; the structured fields stay.
    expect(toCode.builderStream).toBe('vnet_bff_dev');
    expect(toCode.builderAggregation).toBe('avg');
  });
});

describe('QueryEditorSQL / PromQL seeding', () => {
  afterEach(() => jest.clearAllMocks());

  it('seeds the expression when a metric stream is picked in PromQL mode', async () => {
    // Picking a stream left `expr` empty, so OpenObserve answered
    // "invalid promql query" for `query=`.
    const datasource = newDatasource({
      listStreams: jest.fn().mockResolvedValue(['go_goroutine_count', 'go_memory_used']),
    });
    const { onChange } = renderEditor({
      datasource,
      query: { queryType: 'metrics', queryMode: 'promql', expr: '', format: 'timeseries' },
    });
    await waitFor(() =>
      expect(screen.getByTestId('query-editor-stream').querySelectorAll('option').length).toBeGreaterThan(1)
    );

    fireEvent.change(screen.getByTestId('query-editor-stream'), { target: { value: 'go_goroutine_count' } });

    const updated = lastChange(onChange);
    expect(updated.table).toBe('go_goroutine_count');
    expect(updated.expr).toBe('go_goroutine_count');
  });

  it('keeps an expression the user already wrote', async () => {
    const datasource = newDatasource({
      listStreams: jest.fn().mockResolvedValue(['go_goroutine_count']),
    });
    const { onChange } = renderEditor({
      datasource,
      query: { queryType: 'metrics', queryMode: 'promql', expr: 'rate(go_goroutine_count[5m])' },
    });
    await waitFor(() =>
      expect(screen.getByTestId('query-editor-stream').querySelectorAll('option').length).toBeGreaterThan(1)
    );

    fireEvent.change(screen.getByTestId('query-editor-stream'), { target: { value: 'go_goroutine_count' } });

    expect(lastChange(onChange).expr).toBe('rate(go_goroutine_count[5m])');
  });

  it('seeds the expression when switching from SQL to PromQL', async () => {
    const { onChange } = renderEditor({
      query: { queryType: 'metrics', queryMode: 'sql', table: 'go_goroutine_count', expr: '', rawSql: 'SELECT * FROM "go_goroutine_count"' },
    });
    fireEvent.change(screen.getByTestId('query-editor-mode'), { target: { value: 'promql' } });
    const updated = lastChange(onChange);
    expect(updated.queryMode).toBe('promql');
    expect(updated.expr).toBe('go_goroutine_count');
  });
});

describe('QueryEditorSQL / stream type switch', () => {
  afterEach(() => jest.clearAllMocks());

  it('selects the first stream of the new type so the query is runnable', async () => {
    // Leaving the previous stream in place makes the next run fail, because a
    // log stream does not exist in the traces catalogue.
    const datasource = newDatasource({
      listStreams: jest.fn().mockImplementation((type: string) => {
        if (type === 'traces') {
          return Promise.resolve(['prd', 'dev']);
        }
        return Promise.resolve(['vnet_bff_dev', 'alerta']);
      }),
    });
    const { onChange } = renderEditor({
      datasource,
      query: { queryType: 'logs', table: 'vnet_bff_dev', rawSql: 'SELECT * FROM "vnet_bff_dev" LIMIT 20' },
    });
    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('logs'));

    fireEvent.click(screen.getByRole('button', { name: 'Traces' }));

    await waitFor(() => expect(lastChange(onChange).table).toBe('prd'));
    const updated = lastChange(onChange);
    expect(updated.queryType).toBe('traces');
    expect(updated.rawSql).toBe('SELECT * FROM "prd" LIMIT 20');
  });

  it('keeps the rest of the statement when it adopts the first stream', async () => {
    const datasource = newDatasource({
      listStreams: jest.fn().mockImplementation((type: string) =>
        Promise.resolve(type === 'traces' ? ['prd'] : ['logs_stream'])
      ),
    });
    const { onChange } = renderEditor({
      datasource,
      query: { queryType: 'logs', rawSql: 'SELECT count(*) AS "c" FROM "logs_stream" WHERE a = 1 LIMIT 5' },
    });
    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('logs'));

    fireEvent.click(screen.getByRole('button', { name: 'Traces' }));

    await waitFor(() => expect(lastChange(onChange).table).toBe('prd'));
    expect(lastChange(onChange).rawSql).toBe('SELECT count(*) AS "c" FROM "prd" WHERE a = 1 LIMIT 5');
  });

  it('does not touch the stream when the type has no streams', async () => {
    const datasource = newDatasource({
      listStreams: jest.fn().mockImplementation((type: string) =>
        Promise.resolve(type === 'traces' ? [] : ['vnet_bff_dev'])
      ),
    });
    const { onChange } = renderEditor({
      datasource,
      query: { queryType: 'logs', table: 'vnet_bff_dev', rawSql: 'SELECT * FROM "vnet_bff_dev"' },
    });
    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('logs'));

    fireEvent.click(screen.getByRole('button', { name: 'Traces' }));
    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('traces'));

    // Nothing to adopt: the statement keeps its stream rather than being cleared.
    expect(lastChange(onChange).rawSql).toBe('SELECT * FROM "vnet_bff_dev"');
  });

  it('keeps the stream when the list cannot be loaded', async () => {
    const datasource = newDatasource({
      listStreams: jest.fn().mockImplementation((type: string) =>
        type === 'traces' ? Promise.reject(new Error('offline')) : Promise.resolve(['vnet_bff_dev'])
      ),
    });
    const { onChange } = renderEditor({
      datasource,
      query: { queryType: 'logs', table: 'vnet_bff_dev', rawSql: 'SELECT * FROM "vnet_bff_dev"' },
    });
    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('logs'));

    fireEvent.click(screen.getByRole('button', { name: 'Traces' }));
    await waitFor(() => expect(datasource.listStreams).toHaveBeenCalledWith('traces'));

    expect(lastChange(onChange).rawSql).toBe('SELECT * FROM "vnet_bff_dev"');
  });
});

describe('QueryEditorSQL / stale prop handling', () => {
  afterEach(() => jest.clearAllMocks());

  it('keeps the stream type when a stream is picked right after switching it', async () => {
    // Grafana can echo a query back while props.query still holds the previous
    // values; building the next change from that prop used to revert the type.
    const datasource = newDatasource({
      listStreams: jest.fn().mockImplementation((type: string) =>
        Promise.resolve(type === 'traces' ? ['prd'] : ['vnet_bff_dev'])
      ),
    });
    const { onChange } = renderEditor({
      datasource,
      query: { queryType: 'logs', rawSql: 'SELECT * FROM "vnet_bff_dev" LIMIT 20' },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Traces' }));
    await waitFor(() => expect(lastChange(onChange).queryType).toBe('traces'));

    // Traces use the trace editor's own stream picker.
    await waitFor(() =>
      expect(screen.getByTestId('trace-editor-stream').querySelectorAll('option').length).toBeGreaterThan(1)
    );
    fireEvent.change(screen.getByTestId('trace-editor-stream'), { target: { value: 'prd' } });

    const updated = lastChange(onChange);
    expect(updated.queryType).toBe('traces');
    expect(updated.table).toBe('prd');
  });

  it('applies changes in sequence without dropping the previous one', async () => {
    const { onChange, view } = renderEditor({ query: { queryType: 'logs', size: 0 } });

    fireEvent.change(view.container.querySelector<HTMLInputElement>('#query-editor-limit')!, {
      target: { value: '33' },
    });
    await waitFor(() => expect(lastChange(onChange).size).toBe(33));

    await waitFor(() =>
      expect(screen.getByTestId('query-editor-stream').querySelectorAll('option').length).toBeGreaterThan(1)
    );
    fireEvent.change(screen.getByTestId('query-editor-stream'), { target: { value: 'vnet_bff_dev' } });

    const updated = lastChange(onChange);
    // The limit chosen a moment earlier must survive the stream change.
    expect(updated.size).toBe(33);
    expect(updated.table).toBe('vnet_bff_dev');
  });
});
