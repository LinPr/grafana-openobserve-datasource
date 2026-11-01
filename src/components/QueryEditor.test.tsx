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
    RadioButtonGroup: ({ options, value, onChange }: any) => (
      <div role="group" aria-label="stream-type">
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
  const query = { refId: 'A', queryType: 'logs', ...options.query } as OpenObserveQuery;
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
    const { onChange, onRunQuery } = renderEditor({ query: { queryType: 'metrics', queryMode: 'promql', expr: 'up' } });
    await waitFor(() => expect(screen.getByRole('group', { name: 'stream-type' })).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Logs' }));

    const updated = lastChange(onChange);
    expect(updated.queryType).toBe('logs');
    expect(updated.queryMode).toBe('sql');
    expect(onRunQuery).toHaveBeenCalled();
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

  it('keeps the SQL and the table in sync with the format picker', async () => {
    const { onChange, onRunQuery } = renderEditor({ query: { format: 'auto' } });
    await waitFor(() => expect(screen.getByTestId('query-editor-format')).toBeInTheDocument());

    fireEvent.change(screen.getByTestId('query-editor-format'), { target: { value: 'trace' } });

    expect(lastChange(onChange).format).toBe('trace');
    expect(onRunQuery).toHaveBeenCalled();
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
