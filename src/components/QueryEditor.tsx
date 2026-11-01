/*
 * `Select` is deprecated in Grafana 12 in favour of the alpha `Combobox`, but
 * `Combobox` only exists in @grafana/ui >= 12.2 while this plugin supports
 * Grafana >= 10.4. `@grafana/ui` is provided by the Grafana host at runtime, so
 * importing Combobox would crash on every older Grafana. Keep `Select` until
 * the minimum supported Grafana version is raised.
 */
/* eslint-disable @typescript-eslint/no-deprecated */
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import {
  CodeEditor,
  CodeEditorSuggestionItem,
  CodeEditorSuggestionItemKind,
  InlineField,
  InlineSwitch,
  Input,
  RadioButtonGroup,
  Select,
  Stack,
} from '@grafana/ui';

import { OpenObserveDataSource } from '../datasource';
import {
  DEFAULT_QUERY,
  DEFAULT_SQL,
  FORMAT_OPTIONS,
  OpenObserveOptions,
  OpenObserveQuery,
  ResultFormat,
  StreamType,
} from '../types';
import { completionFetchFields, extractFirstTable, retargetStream } from '../utils/completion';

type Props = QueryEditorProps<OpenObserveDataSource, OpenObserveQuery, OpenObserveOptions>;

const STREAM_TYPES: Array<SelectableValue<StreamType>> = [
  { label: 'Logs', value: 'logs' },
  { label: 'Metrics', value: 'metrics' },
  { label: 'Traces', value: 'traces' },
];

const QUERY_MODES: Array<SelectableValue<'sql' | 'promql'>> = [
  { label: 'SQL', value: 'sql', description: 'OpenObserve SQL over _search' },
  { label: 'PromQL', value: 'promql', description: 'Prometheus compatible API' },
];

/**
 * The OpenObserve query editor.
 *
 * Deliberately small: a stream type picker, the query mode (SQL or PromQL), the
 * result format, a stream picker that also drives completion, and the code
 * editor itself. Everything that knows about OpenObserve semantics lives in the
 * Go backend.
 */
export function QueryEditorSQL(props: Props): React.JSX.Element {
  const { datasource, query, onChange, onRunQuery } = props;

  const queryWithDefaults: OpenObserveQuery = useMemo(
    () => ({ ...DEFAULT_QUERY, ...query }) as OpenObserveQuery,
    [query]
  );

  const mode = queryWithDefaults.queryMode ?? 'sql';
  const streamType = queryWithDefaults.queryType ?? 'logs';

  const [streams, setStreams] = useState<Array<SelectableValue<string>>>([]);
  const [streamsError, setStreamsError] = useState<string | undefined>();
  const [fields, setFields] = useState<Array<{ label: string; detail?: string }>>([]);

  // Load the stream list whenever the stream type changes.
  useEffect(() => {
    let cancelled = false;
    datasource
      .listStreams(streamType)
      .then(names => {
        if (cancelled) {
          return;
        }
        setStreamsError(undefined);
        setStreams(names.map(name => ({ label: name, value: name })));
      })
      .catch(error => {
        if (cancelled) {
          return;
        }
        setStreams([]);
        setStreamsError(error?.data?.error ?? error?.message ?? String(error));
      });
    return () => {
      cancelled = true;
    };
  }, [datasource, streamType]);

  // Column completion for the stream referenced by the current SQL.
  useEffect(() => {
    let cancelled = false;
    const table = queryWithDefaults.table ?? extractFirstTable(queryWithDefaults.rawSql ?? '');
    completionFetchFields(datasource, { ...queryWithDefaults, table })
      .then(suggestions => {
        if (!cancelled) {
          setFields(suggestions.filter(s => s.kind === 'field').map(s => ({ label: s.label, detail: s.detail })));
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [datasource, queryWithDefaults]);

  const patch = useCallback(
    (next: Partial<OpenObserveQuery>, run = false) => {
      onChange({ ...queryWithDefaults, ...next });
      if (run) {
        onRunQuery();
      }
    },
    [onChange, onRunQuery, queryWithDefaults]
  );

  const onStreamTypeChange = (value: StreamType) => {
    const nextType = value ?? 'logs';
    if (nextType === streamType) {
      return;
    }
    // Switching the stream type only re-scopes which catalogue the stream
    // picker lists. The statement itself is untouched: it is still valid SQL
    // for whatever stream is named in its FROM clause.
    patch(
      {
        queryType: nextType,
        // PromQL only exists for metrics in OpenObserve.
        queryMode: nextType === 'metrics' ? queryWithDefaults.queryMode ?? 'sql' : 'sql',
      },
      true
    );
  };

  const onModeChange = (option: SelectableValue<'sql' | 'promql'>) => {
    patch({ queryMode: option.value ?? 'sql' }, true);
  };

  const onFormatChange = (option: SelectableValue<ResultFormat>) => {
    patch({ format: option.value ?? 'auto' }, true);
  };

  const onStreamChange = (option: SelectableValue<string>) => {
    const table = option.value;
    if (!table) {
      patch({ table: undefined }, true);
      return;
    }
    // Only the FROM target is rewritten. Everything else in the statement --
    // the projection, WHERE, GROUP BY, LIMIT -- is left byte for byte alone.
    patch({ table, rawSql: retargetStream(queryWithDefaults.rawSql ?? '', table) }, true);
  };

  const onSqlChange = (value: string) => {
    patch({ rawSql: value, table: extractFirstTable(value) ?? queryWithDefaults.table });
  };

  const onExprChange = (value: string) => {
    patch({ expr: value });
  };

  const onSizeChange = (event: React.FormEvent<HTMLInputElement>) => {
    const next = Number(event.currentTarget.value);
    patch({ size: Number.isFinite(next) ? next : 0 });
  };

  const onToggleSSE = (event: React.FormEvent<HTMLInputElement>) => {
    patch({ enableSSE: event.currentTarget.checked }, true);
  };

  const onToggleCache = (event: React.FormEvent<HTMLInputElement>) => {
    patch({ useCache: event.currentTarget.checked }, true);
  };

  const getSuggestions = useCallback((): CodeEditorSuggestionItem[] => {
    const items: CodeEditorSuggestionItem[] = [];
    for (const stream of streams) {
      items.push({
        label: stream.value ?? '',
        kind: CodeEditorSuggestionItemKind.Field,
        insertText: `"${stream.value}"`,
      });
    }
    for (const field of fields) {
      items.push({
        label: field.label,
        kind: CodeEditorSuggestionItemKind.Field,
        detail: field.detail,
        insertText: field.label,
      });
    }
    return items;
  }, [streams, fields]);

  const warning = streamsError ? `Cannot list ${streamType} streams: ${streamsError}` : undefined;

  return (
    <div>
      <Stack direction="row" gap={1} wrap>
        {/*
          A flat segmented control instead of a dropdown: there are only three
          stream types, and switching between them is a frequent action.
        */}
        <InlineField label="Stream type" tooltip="Which OpenObserve stream type the query runs against">
          <RadioButtonGroup<StreamType>
            options={STREAM_TYPES}
            value={streamType}
            onChange={onStreamTypeChange}
          />
        </InlineField>

        {/*
          InlineField clones its child with `{ invalid, disabled, loading }`, so a
          `disabled` set on the child alone would be overwritten with undefined.
          The flag has to live on InlineField itself.
        */}
        <InlineField
          label="Query mode"
          tooltip="SQL uses the OpenObserve search API; PromQL uses its Prometheus compatible API"
          disabled={streamType !== 'metrics'}
        >
          <Select<'sql' | 'promql'>
            inputId="query-editor-mode"
            width={18}
            value={mode}
            options={QUERY_MODES}
            onChange={onModeChange}
          />
        </InlineField>

        <InlineField label="Format" tooltip="How the response is shaped for the panel">
          <Select<ResultFormat>
            inputId="query-editor-format"
            width={18}
            value={queryWithDefaults.format ?? 'auto'}
            options={FORMAT_OPTIONS as Array<SelectableValue<ResultFormat>>}
            onChange={onFormatChange}
          />
        </InlineField>

        <InlineField label="Stream" tooltip="The stream the query selects from" grow>
          <Select<string>
            inputId="query-editor-stream"
            width={30}
            isClearable
            isLoading={streams.length === 0 && !streamsError}
            value={queryWithDefaults.table ?? null}
            options={streams}
            placeholder={streamsError ? 'cannot list streams' : 'search for a stream'}
            onChange={onStreamChange}
          />
        </InlineField>

        <InlineField label="Limit" tooltip="Rows fetched. Empty means: use the SQL LIMIT, else the datasource default">
          <Input
            id="query-editor-limit"
            width={10}
            type="number"
            min={0}
            value={queryWithDefaults.size ?? 0}
            onChange={onSizeChange}
          />
        </InlineField>

        <InlineField label="SSE" tooltip="Stream the results with the _search_stream endpoint">
          <InlineSwitch id="query-editor-sse" value={queryWithDefaults.enableSSE ?? true} onChange={onToggleSSE} />
        </InlineField>

        <InlineField label="Cache" tooltip="Let OpenObserve serve the query from its result cache">
          <InlineSwitch id="query-editor-cache" value={queryWithDefaults.useCache ?? true} onChange={onToggleCache} />
        </InlineField>
      </Stack>

      {warning && (
        <div style={{ color: 'var(--text-warning)', marginBottom: 8 }} role="alert">
          {warning}
        </div>
      )}

      {mode === 'promql' ? (
        <CodeEditor
          value={queryWithDefaults.expr ?? ''}
          language="promql"
          height={200}
          showMiniMap={false}
          showLineNumbers
          onBlur={onExprChange}
          onChange={onExprChange}
        />
      ) : (
        <CodeEditor
          value={queryWithDefaults.rawSql ?? ''}
          language="sql"
          height={200}
          showMiniMap={false}
          showLineNumbers
          onBlur={onSqlChange}
          onChange={onSqlChange}
          getSuggestions={getSuggestions}
        />
      )}

      {!queryWithDefaults.rawSql && mode === 'sql' && (
        <div style={{ color: 'var(--text-secondary)', marginTop: 4 }}>
          Example: <code>{DEFAULT_SQL}</code> — pick a stream above to fill it in.
        </div>
      )}
    </div>
  );
}
