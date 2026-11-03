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
import { InlineField, Input, Select, Stack } from '@grafana/ui';

import { OpenObserveQuery } from '../types';

const AGGREGATIONS: Array<SelectableValue<string>> = [
  { label: 'None (raw rows)', value: '' },
  { label: 'Average', value: 'avg' },
  { label: 'Sum', value: 'sum' },
  { label: 'Minimum', value: 'min' },
  { label: 'Maximum', value: 'max' },
  { label: 'Count', value: 'count' },
  { label: 'p50', value: 'p50' },
  { label: 'p95', value: 'p95' },
  { label: 'p99', value: 'p99' },
];

const INTERVALS: Array<SelectableValue<string>> = [
  { label: '1 minute', value: '1 minute' },
  { label: '5 minutes', value: '5 minute' },
  { label: '15 minutes', value: '15 minute' },
  { label: '1 hour', value: '1 hour' },
  { label: '1 day', value: '1 day' },
];

type Props = {
  query: OpenObserveQuery;
  streams: Array<SelectableValue<string>>;
  fields: string[];
  onChange: (next: Partial<OpenObserveQuery>, run?: boolean) => void;
};

/**
 * The builder: structured inputs instead of a statement.
 *
 * Nothing here writes into `rawSql`. The statement is derived by the backend
 * from these fields, so a user who switches to Code keeps whatever SQL they had
 * written, and a builder query never has a half-generated statement rendered
 * into the editor.
 */
export function QueryBuilder({ query, streams, fields, onChange }: Props): React.JSX.Element {
  const aggregation = query.builderAggregation ?? '';
  const isAggregated = aggregation !== '';
  const needsField = isAggregated && aggregation !== 'count';

  const fieldOptions = useMemo<Array<SelectableValue<string>>>(
    () => fields.map(name => ({ label: name, value: name })),
    [fields]
  );

  const aggregationOptions = useMemo(() => {
    // Grouping and bucketing only make sense once something is aggregated.
    const current = AGGREGATIONS.find(a => a.value === aggregation);
    return current ? AGGREGATIONS : [{ label: 'None (raw rows)', value: '' }, ...AGGREGATIONS.slice(1)];
  }, [aggregation]);

  return (
    <div data-testid="query-builder">
      <Stack direction="row" gap={1} wrap>
        <InlineField label="Stream" tooltip="The stream to read" grow>
          <Select<string>
            inputId="builder-stream"
            width={32}
            isClearable
            value={query.builderStream ?? null}
            options={streams}
            placeholder="pick a stream"
            onChange={option => onChange({ builderStream: option?.value }, false)}
          />
        </InlineField>

        <InlineField label="Aggregation" tooltip="What to compute. None returns raw rows.">
          <Select<string>
            inputId="builder-aggregation"
            width={22}
            value={aggregation}
            options={aggregationOptions}
            onChange={option => onChange({ builderAggregation: (option?.value ?? '') as OpenObserveQuery['builderAggregation'] }, false)}
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

        {isAggregated && (
          <InlineField label="Interval" tooltip="Size of the time bucket">
            <Select<string>
              inputId="builder-interval"
              width={16}
              value={query.builderInterval ?? '5 minute'}
              options={INTERVALS}
              onChange={option => onChange({ builderInterval: option?.value }, false)}
            />
          </InlineField>
        )}

        {isAggregated && (
          <InlineField label="Group by" tooltip="Comma separated columns to group by" grow>
            <Input
              id="builder-groupby"
              width={28}
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
        )}
      </Stack>

      <div style={{ color: 'var(--text-secondary)', marginTop: 4 }}>
        {isAggregated
          ? 'The backend builds the aggregation. Switch to Code to see or edit the generated SQL.'
          : 'Raw rows from the stream. Switch to Code to write your own statement.'}
      </div>
    </div>
  );
}
