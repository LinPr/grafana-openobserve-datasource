// See QueryEditor.tsx: `Select` is kept over the alpha `Combobox` so the plugin
// keeps working on Grafana >= 10.4.
/* eslint-disable @typescript-eslint/no-deprecated */
import React, { useCallback } from 'react';
import { QueryEditorProps } from '@grafana/data';
import { CodeEditor, InlineField, Select } from '@grafana/ui';

import { OpenObserveDataSource } from '../datasource';
import { OpenObserveOptions, OpenObserveQuery } from '../types';
import { extractFirstTable } from '../utils/completion';

type Props = QueryEditorProps<OpenObserveDataSource, OpenObserveQuery, OpenObserveOptions>;

/**
 * Menu used by Grafana's variable editor: the variable value is the first
 * column of the returned rows, so a plain SQL query is enough.
 */
export function VariableQueryEditor({ onChange, query }: Props): React.JSX.Element {
  const onQueryChange = useCallback(
    (rawSql: string) => {
      onChange({
        ...query,
        rawQuery: true,
        queryMode: 'sql',
        // Variables need rows, not a log or graph frame.
        format: 'table',
        rawSql,
        table: extractFirstTable(rawSql) ?? query.table,
      });
    },
    [onChange, query]
  );

  const onStreamTypeChange = (option: { value?: string }) => {
    onChange({ ...query, queryType: (option.value ?? 'logs') as OpenObserveQuery['queryType'] });
  };

  return (
    <div>
      <InlineField label="Stream type" tooltip="Which stream type the variable query reads">
        <Select
          width={20}
          value={query.queryType ?? 'logs'}
          options={[
            { label: 'Logs', value: 'logs' },
            { label: 'Metrics', value: 'metrics' },
            { label: 'Traces', value: 'traces' },
          ]}
          onChange={onStreamTypeChange}
        />
      </InlineField>

      <CodeEditor
        value={query.rawSql ?? ''}
        language="sql"
        height={150}
        showMiniMap={false}
        showLineNumbers
        onBlur={onQueryChange}
        onChange={onQueryChange}
        getSuggestions={() => []}
      />

      <div style={{ color: 'var(--text-secondary)', marginTop: 4 }}>
        The first column of the result becomes the variable value, e.g.{' '}
        <code>SELECT DISTINCT service_name AS &quot;value&quot; FROM &quot;my_stream&quot;</code>
      </div>
    </div>
  );
}

/** Kept as an alias for older imports. */
export const VariableQueryEditorSql = VariableQueryEditor;
