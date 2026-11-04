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
  Input,
  RadioButtonGroup,
  Select,
  Stack,
} from '@grafana/ui';

import { OpenObserveDataSource } from '../datasource';
import {
  DEFAULT_LIMIT,
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

const SIGNAL_LABELS: Record<StreamType, string> = {
  logs: 'Log',
  metrics: 'Metric',
  traces: 'Trace',
};

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
 * Probe results, so a stream is only ever checked once per session.
 *
 * OpenObserve's stream listing does not report row counts, so emptiness can only
 * be learned with a one-row query. Caching keeps that cost to the first switch
 * instead of repeating it on every one.
 */
const streamHasDataCache = new Map<string, boolean>();

/**
 * Returns the first trace stream that actually holds spans.
 *
 * An empty stream is a dead end: the panel shows "0 series returned" and looks
 * broken even though nothing failed. Only the first few candidates are checked,
 * and a failed probe is skipped rather than surfaced -- this is a convenience,
 * not a query.
 */
async function firstTraceStreamWithData(names: string[], datasource: OpenObserveDataSource): Promise<string> {
  const candidates = names.slice(0, 5);
  // Ask about every candidate at once; the probes are one-row queries.
  const results = await Promise.all(
    candidates.map(async name => {
      const key = `traces:${name}`;
      const cached = streamHasDataCache.get(key);
      if (cached !== undefined) {
        return cached;
      }
      try {
        const has = await datasource.streamHasData('traces', name);
        streamHasDataCache.set(key, has);
        return has;
      } catch {
        // Unreachable stream: treat it as empty and move on.
        return false;
      }
    })
  );
  const index = results.findIndex(Boolean);
  return index >= 0 ? candidates[index] : names[0];
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
          /*
           * The first stream of the type, with no probe query: probing every
           * candidate meant a burst of requests on each switch, exactly the
           * traffic this avoids.
           *
           * Traces are the exception. A trace catalogue often starts with
           * streams that hold no spans (an empty `dev` next to the live `prd`),
           * and defaulting to one of those makes a freshly switched panel look
           * broken. The list is small, so the first candidate with data wins --
           * one probe, done once per switch.
           */
          const table =
            streamType === 'traces' ? await firstTraceStreamWithData(names, datasource) : names[0];
          const merged = {
            ...latestQuery.current,
            table,
            // No retargeting: the previous type's statement was cleared on the
            // switch and must not come back under a stream of another type.
            rawSql: '',
            editorMode: latestQuery.current.editorMode ?? 'builder',
            // The new type's default stream is in place, so the query is
            // runnable again -- but only a click on Run query should send it.
            deferRun: false,
          };
          latestQuery.current = merged;
          onChange(merged);
          // Deliberately no run here. Switching stream type changes which
          // parameters are meaningful, so a query fired at this instant is
          // half-built and typically fails -- the user gets an error banner for
          // something they did not ask to run.
        } else if (pendingAutoSelect.current) {
          // The new type has no streams to adopt. Nothing is runnable, but the
          // panel must not keep showing the previous type's results.
          pendingAutoSelect.current = false;
          const cleared = { ...latestQuery.current, table: undefined, rawSql: '', deferRun: false };
          latestQuery.current = cleared;
          onChange(cleared);
        }
      })
      .catch(error => {
        if (cancelled) {
          return;
        }
        pendingAutoSelect.current = false;
        setStreams([]);
        setStreamsError(error?.data?.error ?? error?.message ?? String(error));
        // Nothing to adopt, but the panel should still not sit on stale results.
        const cleared = { ...latestQuery.current, deferRun: false };
        latestQuery.current = cleared;
        onChange(cleared);
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
        // The previous type's statement cannot run against the new type, so it
        // goes away too. With nothing to run the panel shows "No data" rather
        // than results belonging to a stream type the user just left.
        rawSql: '',
        expr: '',
        /*
         * The stream has to go as well. Leaving it meant the new type's query
         * carried a stream from the old catalogue -- switching from metrics to
         * traces asked OpenObserve for `go_memory_used` in the trace directory
         * and failed with "Search stream not found". The effect below adopts a
         * stream from the new type's list.
         */
        table: undefined,
        builderStream: undefined,
        // Traces need a query type; default to the list view so the form is
        // coherent as soon as the editor renders.
        traceMode: nextType === 'traces' ? queryWithDefaults.traceMode ?? 'search' : queryWithDefaults.traceMode,
        // Pinned so clearing the statement cannot flip the editor to Code: the
        // mode is derived from "is there a statement", so it has to be explicit.
        editorMode: editorMode,
        size: queryWithDefaults.size ?? DEFAULT_LIMIT,
        deferRun: true,
      },
      // Run once with deferRun set: the datasource answers with an empty result
      // instead of sending anything, which is what actually clears the panel.
      true
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
    if (editorMode === 'builder') {
      // The builder derives its statement from these fields, so the limit is
      // just a number here. Writing it into rawSql would look like a code query
      // and flip the editor out of Builder.
      patch({ size: limit });
      return;
    }
    // Keep the SQL in step with the field, so the panel and the statement never
    // disagree about how many rows to fetch.
    patch({ size: limit, rawSql: setSqlLimit(queryWithDefaults.rawSql ?? '', limit) });
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

  /*
   * OpenObserve has one catalogue per signal, so "Stream" on its own is
   * ambiguous once the type can be logs, metrics or traces. The labels name the
   * signal to keep the editor unambiguous.
   */
  const streamLabel = `${SIGNAL_LABELS[streamType]} stream`;

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
        <InlineField label={streamLabel} tooltip="The stream the query selects from" grow>
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

      </Stack>

      {streamType !== 'traces' && (
        <Stack direction="row" gap={1} wrap>
          <InlineField label="Limit" tooltip="Maximum rows to return">
            <Input
              id="query-editor-limit"
              width={10}
              type="number"
              min={0}
              value={queryWithDefaults.size ?? DEFAULT_LIMIT}
              onChange={onSizeChange}
            />
          </InlineField>
        </Stack>
      )}

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
          streamLabel={streamLabel}
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
