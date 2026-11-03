/*
 * `Select` is deprecated in Grafana 12 in favour of the alpha `Combobox`, but
 * `Combobox` only exists in @grafana/ui >= 12.2 while this plugin supports
 * Grafana >= 10.4. `@grafana/ui` is provided by the Grafana host at runtime, so
 * importing Combobox would crash on every older Grafana. Keep `Select` until
 * the minimum supported Grafana version is raised.
 */
/* eslint-disable @typescript-eslint/no-deprecated */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
  EditorMode,
  OpenObserveOptions,
  OpenObserveQuery,
  StreamType,
} from '../types';
import { completionFetchFields, extractFirstTable, extractLimit, retargetStream, setSqlLimit } from '../utils/completion';
import { QueryBuilder } from './QueryBuilder';
import { TraceQueryEditor } from './TraceQueryEditor';

type Props = QueryEditorProps<OpenObserveDataSource, OpenObserveQuery, OpenObserveOptions>;

const STREAM_TYPES: Array<SelectableValue<StreamType>> = [
  { label: 'Logs', value: 'logs' },
  { label: 'Metrics', value: 'metrics' },
  { label: 'Traces', value: 'traces' },
];

const EDITOR_MODES: Array<SelectableValue<EditorMode>> = [
  { label: 'Builder', value: 'builder', description: 'Pick streams and aggregations; the SQL is generated' },
  { label: 'Code', value: 'code', description: 'Write the query yourself' },
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
/**
 * Chooses the stream to adopt when the user switches the stream type.
 *
 * The catalogue lists every stream of a type, including ones that are stale or
 * empty, so the first name can be a dead end -- the trace catalogue here starts
 * with "dev" (no rows) while "prd" is the live one. This probes at most a
 * handful of candidates and returns the first with data, falling back to the
 * first name so the picker is never left empty.
 */
async function pickDefaultStream(
  names: string[],
  streamType: StreamType,
  datasource: OpenObserveDataSource,
  range?: { from: number; to: number }
): Promise<string> {
  const candidates = names.slice(0, 5);
  for (const name of candidates) {
    try {
      const has = await datasource.streamHasData(streamType, name, range);
      if (has) {
        return name;
      }
    } catch {
      // Probing is best effort; fall through to the next candidate.
    }
  }
  return names[0];
}

export function QueryEditorSQL(props: Props): React.JSX.Element {
  const { datasource, query, onChange, onRunQuery } = props;

  const queryWithDefaults: OpenObserveQuery = useMemo(
    () => ({ ...DEFAULT_QUERY, ...query }) as OpenObserveQuery,
    [query]
  );

  /*
   * Grafana echoes a just-sent query back a moment later -- typically when the
   * CodeEditor reports its content after the stream picker rewrote the FROM
   * target. At that instant `props.query` can still hold the values from
   * *before* the last change, so building the next query from `queryWithDefaults`
   * silently reverted fields the user had just set (picking a stream after
   * switching to Traces put the type back to Logs).
   *
   * Keeping the newest query in a ref makes each change build on the last one
   * we produced, not on a possibly stale prop.
   */
  const latestQuery = useRef<OpenObserveQuery>(queryWithDefaults);
  useEffect(() => {
    // A prop update that differs from what we last sent means the parent is
    // authoritative again (dashboard load, JSON edit, ...), so take it.
    latestQuery.current = queryWithDefaults;
  }, [queryWithDefaults]);

  const mode = queryWithDefaults.queryMode ?? 'sql';
  const streamType = queryWithDefaults.queryType ?? 'logs';
  /*
   * A saved query with a statement but no explicit mode is code: it was written
   * by hand, and opening it in the builder would hide it. Everything else
   * defaults to the builder, which is the friendlier starting point.
   */
  const editorMode: EditorMode =
    queryWithDefaults.editorMode ?? (queryWithDefaults.rawSql?.trim() ? 'code' : 'builder');

  const [streams, setStreams] = useState<Array<SelectableValue<string>>>([]);
  const [streamsError, setStreamsError] = useState<string | undefined>();
  const [fields, setFields] = useState<Array<{ label: string; detail?: string }>>([]);

  /*
   * Set when the user switches the stream type. The stream list is loaded
   * asynchronously, so the "pick the first stream of the new type" step has to
   * happen once that list arrives -- doing it in the click handler would race
   * the fetch and pick from the previous type's list.
   */
  const pendingAutoSelect = useRef(false);

  // Load the stream list whenever the stream type changes.
  useEffect(() => {
    let cancelled = false;
    datasource
      .listStreams(streamType)
      .then(async names => {
        if (cancelled) {
          return;
        }
        setStreamsError(undefined);
        setStreams(names.map(name => ({ label: name, value: name })));

        // Switching type leaves the query pointing at a stream of the old type,
        // which errors on the next run. Adopt the first stream of the new type
        // so the editor is immediately runnable.
        if (pendingAutoSelect.current && names.length > 0) {
          pendingAutoSelect.current = false;
          // Trace catalogues list streams that may be empty (stale or unused),
          // so the first name is not necessarily a useful default. Prefer a
          // stream that actually has data in the panel's time range.
          const table = await pickDefaultStream(names, streamType, datasource);
          const merged = { ...latestQuery.current, table, rawSql: retargetStream(latestQuery.current.rawSql ?? '', table) };
          latestQuery.current = merged;
          onChange(merged);
          // The type switch itself cannot run the query (the stream is only
          // valid once this list has loaded), so run it here instead: switching
          // type must refresh the panel rather than leave stale results.
          onRunQuery();
        }
      })
      .catch(error => {
        if (cancelled) {
          return;
        }
        pendingAutoSelect.current = false;
        setStreams([]);
        setStreamsError(error?.data?.error ?? error?.message ?? String(error));
        onRunQuery();
      });
    return () => {
      cancelled = true;
    };
    // onChange is stable from Grafana; listing it would refetch on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
      const merged = { ...latestQuery.current, ...next } as OpenObserveQuery;
      latestQuery.current = merged;
      onChange(merged);
      if (run) {
        onRunQuery();
      }
    },
    [onChange, onRunQuery]
  );

  const onStreamTypeChange = (value: StreamType) => {
    const nextType = value ?? 'logs';
    if (nextType === streamType) {
      return;
    }
    // The statement keeps pointing at a stream of the previous type, which
    // would fail on the next run, so the effect above adopts the first stream
    // of the new type as soon as its list has loaded.
    pendingAutoSelect.current = true;
    patch(
      {
        queryType: nextType,
        // PromQL only exists for metrics in OpenObserve.
        queryMode: nextType === 'metrics' ? queryWithDefaults.queryMode ?? 'sql' : 'sql',
      },
      false
    );
  };

  const onEditorModeChange = (next: EditorMode) => {
    // Switching modes only flips the flag: neither the builder fields nor the
    // written statement are touched, so going back and forth loses nothing.
    patch({ editorMode: next }, true);
  };

  const onModeChange = (option: SelectableValue<'sql' | 'promql'>) => {
    const nextMode = option.value ?? 'sql';
    // Same reasoning as onStreamChange: PromQL needs a metric to be valid, and
    // the currently picked stream is the obvious candidate.
    const expr = nextMode === 'promql' && !queryWithDefaults.expr?.trim() ? queryWithDefaults.table : queryWithDefaults.expr;
    patch({ queryMode: nextMode, expr }, true);
  };

  const onStreamChange = (option: SelectableValue<string>) => {
    const table = option.value;
    if (!table) {
      patch({ table: undefined }, true);
      return;
    }
    if (mode === 'promql') {
      // A PromQL query *is* the metric name, so picking a stream seeds the
      // expression. Leaving it empty sent `query=` and OpenObserve rejected it
      // with "invalid promql query".
      patch({ table, expr: queryWithDefaults.expr?.trim() ? queryWithDefaults.expr : table }, true);
      return;
    }
    // Only the FROM target is rewritten. Everything else in the statement --
    // the projection, WHERE, GROUP BY, LIMIT -- is left byte for byte alone.
    patch({ table, rawSql: retargetStream(queryWithDefaults.rawSql ?? '', table) }, true);
  };

  /*
   * The builder keeps its stream in `builderStream` so it never rewrites the
   * statement's FROM target. `table` is mirrored only for field completion.
   */
  const onBuilderChange = useCallback(
    (next: Partial<OpenObserveQuery>, run = false) => {
      const withTable =
        next.builderStream !== undefined ? { ...next, table: next.builderStream ?? undefined } : next;
      patch(withTable, run);
    },
    [patch]
  );

  const onSqlChange = (value: string) => {
    // ... and the other way round: a LIMIT typed into the editor updates the field.
    patch({
      rawSql: value,
      table: extractFirstTable(value) ?? queryWithDefaults.table,
      size: extractLimit(value),
    });
  };

  const onExprChange = (value: string) => {
    patch({ expr: value });
  };

  const onSizeChange = (event: React.FormEvent<HTMLInputElement>) => {
    const next = Number(event.currentTarget.value);
    const limit = Number.isFinite(next) ? next : 0;
    // Keep the SQL in step with the field, so the panel and the statement never
    // disagree about how many rows to fetch.
    patch({ size: limit, rawSql: setSqlLimit(queryWithDefaults.rawSql ?? '', limit) });
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

        {streamType !== 'traces' && (
        <InlineField
          label="Editor"
          htmlFor="query-editor-mode-toggle"
          tooltip="Builder picks streams and aggregations; Code is the statement you write"
        >
          <RadioButtonGroup<EditorMode>
            options={EDITOR_MODES}
            value={editorMode}
            onChange={onEditorModeChange}
            aria-label="editor-mode"
          />
        </InlineField>
        )}

        {streamType !== 'traces' && editorMode === 'code' && (
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
        )}

        {streamType !== 'traces' && (
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
        )}

        {streamType !== 'traces' && (
          <>
            <InlineField label="SSE" tooltip="Stream the results with the _search_stream endpoint">
              <InlineSwitch id="query-editor-sse" value={queryWithDefaults.enableSSE ?? true} onChange={onToggleSSE} />
            </InlineField>

            <InlineField label="Cache" tooltip="Let OpenObserve serve the query from its result cache">
              <InlineSwitch id="query-editor-cache" value={queryWithDefaults.useCache ?? true} onChange={onToggleCache} />
            </InlineField>
          </>
        )}
      </Stack>

      {warning && (
        <div style={{ color: 'var(--text-warning)', marginBottom: 8 }} role="alert">
          {warning}
        </div>
      )}

      {/*
        Traces get their own Jaeger-style form: the three query types need
        different inputs, and the backend builds the SQL from them, so there is
        no statement for the user to write.
      */}
      {streamType === 'traces' && <TraceQueryEditor datasource={datasource} query={queryWithDefaults} onChange={onChange} onRunQuery={onRunQuery} />}

      {streamType === 'traces' ? null : editorMode === 'builder' ? (
        // The builder renders the derived query preview, never the statement,
        // so a hand-written SQL is never overwritten by an aggregation choice.
        <QueryBuilder
          query={queryWithDefaults}
          streams={streams}
          fields={fields.map(f => f.label)}
          onChange={onBuilderChange}
        />
      ) : mode === 'promql' ? (
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

      {streamType !== 'traces' && editorMode === 'code' && mode === 'sql' && !queryWithDefaults.rawSql && (
        <div style={{ color: 'var(--text-secondary)', marginTop: 4 }}>
          Example: <code>{DEFAULT_SQL}</code> — pick a stream above to fill it in.
        </div>
      )}
    </div>
  );
}
