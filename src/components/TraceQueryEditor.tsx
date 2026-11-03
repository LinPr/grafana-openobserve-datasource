// See QueryEditor.tsx: `Select` is kept over the alpha `Combobox` so the plugin
// keeps working on Grafana >= 10.4.
/* eslint-disable @typescript-eslint/no-deprecated */
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { SelectableValue } from '@grafana/data';
import { InlineField, Input, RadioButtonGroup, Select, Stack } from '@grafana/ui';

import { OpenObserveDataSource } from '../datasource';
import { OpenObserveQuery, TraceMode } from '../types';

/**
 * The trace query form, modelled on Jaeger's editor: three query types that
 * each map to one SQL query the backend builds.
 *
 * Search  -- list the traces that match the filters
 * TraceID -- load every span of one trace
 * Graph   -- service-to-service call counts derived from span parent/child links
 */
export interface TraceQueryEditorProps {
  datasource: OpenObserveDataSource;
  query: OpenObserveQuery;
  onChange: (query: OpenObserveQuery) => void;
  onRunQuery: () => void;
}

/** How many traces a search lists by default. */
const DEFAULT_TRACE_LIMIT = 100;

const TRACE_MODES: Array<SelectableValue<TraceMode>> = [
  { label: 'Search', value: 'search', description: 'List the traces that match the filters' },
  { label: 'TraceID', value: 'traceId', description: 'Load every span of one trace' },
  { label: 'Graph', value: 'graph', description: 'Service-to-service call counts' },
];

export function TraceQueryEditor({ datasource, query, onChange, onRunQuery }: TraceQueryEditorProps) {
  const mode: TraceMode = query.traceMode ?? 'search';
  const stream = query.table ?? '';

  const [streams, setStreams] = useState<Array<SelectableValue<string>>>([]);
  const [services, setServices] = useState<Array<SelectableValue<string>>>([]);
  const [operations, setOperations] = useState<Array<SelectableValue<string>>>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | undefined>();

  const patch = useCallback(
    (next: Partial<OpenObserveQuery>, run = false) => {
      onChange({ ...query, ...next });
      if (run) {
        onRunQuery();
      }
    },
    [onChange, onRunQuery, query]
  );

  // Trace streams for the picker.
  useEffect(() => {
    let cancelled = false;
    datasource
      .listStreams('traces')
      .then(names => {
        if (!cancelled) {
          setStreams(names.map(name => ({ label: name, value: name })));
        }
      })
      .catch(e => {
        if (!cancelled) {
          setStreams([]);
          setError(e?.data?.error ?? e?.message ?? String(e));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [datasource]);

  // Service / operation suggestions come from the trace stream schema itself.
  useEffect(() => {
    if (!stream) {
      setServices([]);
      setOperations([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    datasource
      .distinctTraceValues(stream, 'service_name')
      .then(values => {
        if (!cancelled) {
          setServices(values.map(v => ({ label: v, value: v })));
          setError(undefined);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setServices([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    datasource
      .distinctTraceValues(stream, 'operation_name')
      .then(values => {
        if (!cancelled) {
          setOperations(values.map(v => ({ label: v, value: v })));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setOperations([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [datasource, stream]);

  const serviceOptions = useMemo(() => [{ label: 'All', value: '' }, ...services], [services]);
  const operationOptions = useMemo(() => [{ label: 'All', value: '' }, ...operations], [operations]);

  return (
    <div>
      {/*
        Laid out like Jaeger's editor: which trace stream to read and what the
        query returns come first, then one row per query type. Keeping the
        filters on their own rows means the labels stay readable instead of
        wrapping into each other.
      */}
      <Stack direction="row" gap={1} wrap>
        <InlineField label="Query type" tooltip="What the trace query returns">
          <RadioButtonGroup<TraceMode>
            options={TRACE_MODES}
            value={mode}
            onChange={value => patch({ traceMode: value ?? 'search' }, true)}
            aria-label="trace-query-type"
          />
        </InlineField>

        <InlineField label="Trace stream" tooltip="The trace stream holding the spans">
          <Select<string>
            inputId="trace-editor-stream"
            width={22}
            value={stream || null}
            options={streams}
            isLoading={streams.length === 0 && !error}
            placeholder={error ? 'cannot list streams' : 'pick a trace stream'}
            onChange={option => patch({ table: option.value ?? undefined }, true)}
          />
        </InlineField>
      </Stack>

      {mode === 'search' && (
        <>
          <Stack direction="row" gap={1} wrap>
            <InlineField label="Service" tooltip="Only traces whose entry span belongs to this service">
              <Select<string>
                inputId="trace-editor-service"
                width={24}
                isClearable
                value={query.serviceName ?? ''}
                options={serviceOptions}
                isLoading={loading}
                placeholder="All"
                onChange={option => patch({ serviceName: option.value ?? '' })}
              />
            </InlineField>

            <InlineField label="Operation" tooltip="Only traces whose entry span has this name">
              <Select<string>
                inputId="trace-editor-operation"
                width={26}
                isClearable
                value={query.operationName ?? ''}
                options={operationOptions}
                placeholder="All"
                onChange={option => patch({ operationName: option.value ?? '' })}
              />
            </InlineField>

            <InlineField label="Min duration" tooltip='e.g. "1.2s", "100ms", "500us"'>
              <Input
                id="trace-editor-min-duration"
                width={14}
                placeholder="e.g. 100ms"
                value={query.minDuration ?? ''}
                onChange={event => patch({ minDuration: event.currentTarget.value })}
              />
            </InlineField>

            <InlineField label="Max duration" tooltip='e.g. "1.2s", "100ms", "500us"'>
              <Input
                id="trace-editor-max-duration"
                width={14}
                placeholder="e.g. 1.2s"
                value={query.maxDuration ?? ''}
                onChange={event => patch({ maxDuration: event.currentTarget.value })}
              />
            </InlineField>
          </Stack>

          <Stack direction="row" gap={1} wrap>
            <InlineField label="Attributes" grow tooltip="Space separated key=value filters, e.g. http.status_code=200">
              <Input
                id="trace-editor-tags"
                placeholder="error=true http.status_code=200"
                value={(query.traceTags ?? []).join(' ')}
                onChange={event =>
                  patch({
                    traceTags: event.currentTarget.value
                      .split(/\s+/)
                      .map(t => t.trim())
                      .filter(Boolean),
                  })
                }
              />
            </InlineField>

          </Stack>
        </>
      )}

      {mode === 'traceId' && (
        <InlineField label="Trace ID" grow tooltip="Loads every span of this trace">
          <Input
            id="trace-editor-trace-id"
            placeholder="enter a trace id"
            value={query.traceId ?? ''}
            onChange={event => patch({ traceId: event.currentTarget.value })}
            onKeyDown={event => {
              if (event.key === 'Enter') {
                onRunQuery();
              }
            }}
          />
        </InlineField>
      )}

      {mode === 'graph' && (
        <Stack direction="row" gap={1} wrap>
          <InlineField label="Service" tooltip="Only edges touching this service, in either direction">
            <Select<string>
              inputId="trace-editor-graph-service"
              width={24}
              isClearable
              value={query.serviceName ?? ''}
              options={serviceOptions}
              placeholder="All"
              onChange={option => patch({ serviceName: option.value ?? '' })}
            />
          </InlineField>
        </Stack>
      )}

      <Stack direction="row" gap={1} wrap>
            <InlineField label="Limit" tooltip="How many traces to list">
              <Input
                id="trace-editor-limit"
                width={12}
                type="number"
                min={0}
                value={query.size ?? DEFAULT_TRACE_LIMIT}
                onChange={event => {
                  const next = Number(event.currentTarget.value);
                  patch({ size: Number.isFinite(next) ? next : 0 });
                }}
              />
            </InlineField>
      </Stack>

      {error && (
        <div role="alert" style={{ color: 'var(--text-warning)', marginTop: 4 }}>
          {error}
        </div>
      )}

      {mode === 'graph' && !error && (
        <div style={{ color: 'var(--text-secondary)', marginTop: 4 }}>
          Caller → callee edges derived from span parent/child links, drawn as a node graph. Services come from
          OpenObserve&apos;s inferred service name, so client spans attribute to the system they call.
        </div>
      )}
    </div>
  );
}
