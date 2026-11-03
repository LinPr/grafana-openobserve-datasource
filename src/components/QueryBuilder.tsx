/*
 * `Select` is deprecated in Grafana 12 in favour of the alpha `Combobox`, but
 * `Combobox` only exists in @grafana/ui >= 12.2 while this plugin supports
 * Grafana >= 10.4. `@grafana/ui` is provided by the Grafana host at runtime, so
 * importing Combobox would crash on every older Grafana. Keep `Select` until
 * the minimum supported Grafana version is raised.
 */
/* eslint-disable @typescript-eslint/no-deprecated */
import React, { useMemo } from 'react';
import { SelectableValue } from '@grafana/data';
import { InlineField, Input, RadioButtonGroup, Select } from '@grafana/ui';

import { BuilderAggregation, OpenObserveQuery } from '../types';

const AGGREGATIONS: Array<SelectableValue<string>> = [
  { label: 'Raw rows', value: '', description: 'Return the rows as stored' },
  { label: 'Count', value: 'count', description: 'count(*)' },
  { label: 'Average', value: 'avg' },
  { label: 'Sum', value: 'sum' },
  { label: 'Minimum', value: 'min' },
  { label: 'Maximum', value: 'max' },
  { label: 'p50', value: 'p50' },
  { label: 'p95', value: 'p95' },
  { label: 'p99', value: 'p99' },
];

const INTERVALS: Array<SelectableValue<string>> = [
  { label: '1 second', value: '1 second' },
  { label: '10 seconds', value: '10 second' },
  { label: '1 minute', value: '1 minute' },
  { label: '5 minutes', value: '5 minute' },
  { label: '15 minutes', value: '15 minute' },
  { label: '1 hour', value: '1 hour' },
  { label: '1 day', value: '1 day' },
];

const AGGREGATION_MODES: Array<SelectableValue<'raw' | 'aggregate'>> = [
  { label: 'Raw', value: 'raw' },
  { label: 'Aggregate', value: 'aggregate' },
];

type Props = {
  query: OpenObserveQuery;
  streams: Array<SelectableValue<string>>;
  fields: string[];
  onChange: (next: Partial<OpenObserveQuery>, run?: boolean) => void;
};

/**
 * The builder, laid out like the ClickHouse and Prometheus editors: the inputs
 * that shape the query come first, and a read-only preview of the statement
 * follows so it is obvious what will run without putting generated text into
 * the Code editor.
 *
 * Nothing here writes into `rawSql`.
 */
export function QueryBuilder({ query, streams, fields, onChange }: Props): React.JSX.Element {
  const aggregation = query.builderAggregation ?? '';
  const isAggregated = aggregation !== '';
  const needsField = isAggregated && aggregation !== 'count';

  const fieldOptions = useMemo<Array<SelectableValue<string>>>(
    () => fields.map(name => ({ label: name, value: name })),
    [fields]
  );

  const setAggregation = (value: BuilderAggregation | '') =>
    onChange(
      {
        builderAggregation: value,
        // Clear a field the new aggregation cannot use, so the preview never
        // shows a column that would be ignored.
        builderField: value === 'count' || value === '' ? undefined : query.builderField,
      },
      false
    );

  return (
    <div data-testid="query-builder">
      <InlineField label="Stream" tooltip="The stream to read" grow>
        <Select<string>
          inputId="builder-stream"
          width={40}
          isClearable
          value={query.builderStream ?? null}
          options={streams}
          placeholder="pick a stream"
          onChange={option => onChange({ builderStream: option?.value }, false)}
        />
      </InlineField>

      <InlineField label="Query" tooltip="Raw rows, or an aggregation over time">
        <RadioButtonGroup<'raw' | 'aggregate'>
          options={AGGREGATION_MODES}
          value={isAggregated ? 'aggregate' : 'raw'}
          onChange={mode =>
            // Picking "Aggregate" starts from count(*) so the query is valid
            // straight away instead of waiting for a field.
            setAggregation(mode === 'aggregate' ? aggregation || 'count' : '')
          }
          aria-label="builder-query-kind"
        />
      </InlineField>

      {isAggregated && (
        <>
          <InlineField label="Function" tooltip="The aggregation to compute">
            <Select<string>
              inputId="builder-aggregation"
              width={16}
              value={aggregation}
              options={AGGREGATIONS.filter(a => a.value !== '')}
              onChange={option => setAggregation((option?.value ?? '') as BuilderAggregation)}
            />
          </InlineField>

          {needsField && (
            <InlineField label="Field" tooltip="The column to aggregate">
              <Select<string>
                inputId="builder-field"
                width={24}
                isClearable
                value={query.builderField ?? null}
                options={fieldOptions}
                placeholder="pick a field"
                onChange={option => onChange({ builderField: option?.value }, false)}
              />
            </InlineField>
          )}

          <InlineField label="Interval" tooltip="Size of the time bucket">
            <Select<string>
              inputId="builder-interval"
              width={16}
              value={query.builderInterval ?? '1 minute'}
              options={INTERVALS}
              onChange={option => onChange({ builderInterval: option?.value }, false)}
            />
          </InlineField>

          <InlineField label="Group by" tooltip="Comma separated columns to group by" grow>
            <Input
              id="builder-groupby"
              width={30}
              value={(query.builderGroupBy ?? []).join(', ')}
              placeholder="e.g. service_name"
              onChange={event =>
                onChange(
                  {
                    builderGroupBy: event.currentTarget.value
                      .split(',')
                      .map(part => part.trim())
                      .filter(Boolean),
                  },
                  false
                )
              }
            />
          </InlineField>
        </>
      )}

      <QueryPreview query={query} />
    </div>
  );
}

/**
 * A read-only rendering of what the builder will run.
 *
 * It mirrors the backend's statement so the query is never a black box, and it
 * lives outside the Code editor so switching modes neither depends on nor
 * overwrites it. Rendered as plain monospace text rather than a CodeEditor: an
 * editor-shaped box reads as "type here", and this cannot be typed into.
 */
function QueryPreview({ query }: { query: OpenObserveQuery }): React.JSX.Element | null {
  const stream = query.builderStream?.trim();
  const aggregation = query.builderAggregation ?? '';
  if (!stream) {
    return <div style={{ color: 'var(--text-secondary)', marginTop: 8 }}>Pick a stream to see the query.</div>;
  }

  const limit = query.size ?? 100;
  const quoted = `"${stream.replace(/"/g, '""')}"`;
  let sql: string;
  if (!aggregation) {
    sql = `SELECT * FROM ${quoted} LIMIT ${limit}`;
  } else {
    const interval = query.builderInterval ?? '1 minute';
    const field = query.builderField?.trim();
    const value = aggregation === 'count' && !field ? 'count(*)' : `${aggregation}("${field ?? ''}")`;
    const groups = ['ts', ...(query.builderGroupBy ?? []).map(g => `"${g}"`)].join(', ');
    sql = `SELECT histogram(_timestamp, '${interval}') AS ts, ${value} AS value FROM ${quoted} GROUP BY ${groups} ORDER BY ts LIMIT ${limit}`;
  }

  return (
    <div
      data-testid="builder-preview"
      style={{
        marginTop: 8,
        padding: '8px 12px',
        borderRadius: 2,
        background: 'var(--background-color-secondary)',
        border: '1px solid var(--border-weak)',
        fontFamily: 'monospace',
        fontSize: 12,
        color: 'var(--text-secondary)',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
      }}
    >
      {sql}
    </div>
  );
}
