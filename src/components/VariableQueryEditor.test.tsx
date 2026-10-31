import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { CoreApp } from '@grafana/data';

import { VariableQueryEditor } from './VariableQueryEditor';
import { OpenObserveQuery } from '../types';

// Monaco cannot render in jsdom; a textarea keeps the component's behaviour
// (value in, onChange out) testable.
jest.mock('@grafana/ui', () => {
  const actual = jest.requireActual('@grafana/ui');
  return {
    ...actual,
    CodeEditor: ({ value, language, onChange }: any) => (
      <textarea
        aria-label={`code-${language}`}
        data-language={language}
        value={value}
        onChange={event => onChange(event.currentTarget.value)}
      />
    ),
    // The real Select needs IntersectionObserver, which jsdom does not provide.
    Select: ({ value, options, onChange }: any) => (
      <select
        aria-label="stream-type"
        value={value ?? ''}
        onChange={event => {
          const option = (options ?? []).find((o: any) => String(o.value) === event.currentTarget.value);
          if (option) {
            onChange(option);
          }
        }}
      >
        {(options ?? []).map((option: any) => (
          <option key={String(option.value)} value={String(option.value)}>
            {option.label}
          </option>
        ))}
      </select>
    ),
  };
});

/**
 * The variable editor turns whatever the user types into a table-formatted SQL
 * query, because Grafana takes the first column of the result as the variable
 * value.
 */
function renderVariableEditor(query: Partial<OpenObserveQuery> = {}) {
  const onChange = jest.fn();
  const full = { refId: 'A', queryType: 'logs', ...query } as OpenObserveQuery;

  const props = {
    datasource: {} as never,
    query: full,
    onChange,
    onRunQuery: jest.fn(),
    onBlur: () => undefined,
    data: undefined,
    range: undefined,
    app: CoreApp.Dashboard,
    queries: [full],
  } as unknown as Parameters<typeof VariableQueryEditor>[0];

  render(<VariableQueryEditor {...props} />);

  return { onChange, query: full };
}

const lastQuery = (onChange: jest.Mock): OpenObserveQuery => onChange.mock.calls[onChange.mock.calls.length - 1][0];

describe('VariableQueryEditor', () => {
  afterEach(() => jest.clearAllMocks());

  it('renders the stored SQL', () => {
    renderVariableEditor({ rawSql: 'SELECT DISTINCT service_name FROM "logs"' });
    expect(screen.getByLabelText('code-sql')).toHaveValue('SELECT DISTINCT service_name FROM "logs"');
  });

  it('forces the table format so Grafana can read the first column', () => {
    const { onChange } = renderVariableEditor({ rawSql: 'SELECT 1' });

    fireEvent.change(screen.getByLabelText('code-sql'), {
      target: { value: 'SELECT DISTINCT service_name AS "value" FROM "vnet_bff_dev"' },
    });

    const updated = lastQuery(onChange);
    expect(updated.format).toBe('table');
    expect(updated.queryMode).toBe('sql');
  });

  it('tracks the stream referenced by the SQL', () => {
    const { onChange } = renderVariableEditor();

    fireEvent.change(screen.getByLabelText('code-sql'), {
      target: { value: 'SELECT DISTINCT severity FROM "alerta"' },
    });

    expect(lastQuery(onChange).table).toBe('alerta');
  });

  it('keeps the previous stream when the SQL has no FROM clause', () => {
    const { onChange } = renderVariableEditor({ table: 'alerta' });

    fireEvent.change(screen.getByLabelText('code-sql'), { target: { value: 'SELECT 1' } });

    expect(lastQuery(onChange).table).toBe('alerta');
  });

  it('marks the query as a raw query', () => {
    const { onChange } = renderVariableEditor();
    fireEvent.change(screen.getByLabelText('code-sql'), { target: { value: 'SELECT 1' } });
    expect(lastQuery(onChange).rawQuery).toBe(true);
  });

  it('changes the stream type', () => {
    const { onChange } = renderVariableEditor({ queryType: 'logs' });

    fireEvent.change(screen.getByLabelText('stream-type'), { target: { value: 'metrics' } });

    expect(lastQuery(onChange).queryType).toBe('metrics');
  });
});
