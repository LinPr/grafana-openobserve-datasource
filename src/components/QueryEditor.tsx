import React from 'react';
import { QueryEditorProps } from '@grafana/data';

import { OpenObserveDataSource } from '../datasource';
import { OpenObserveOptions, OpenObserveQuery } from '../types';
import { EditorMode, SqlDatasource, SqlQueryEditor } from '@grafana/plugin-ui';
import { Combobox, InlineField, ComboboxOption, Stack, InlineSwitch } from '@grafana/ui';

type Props = QueryEditorProps<OpenObserveDataSource, OpenObserveQuery, OpenObserveOptions>;

export function QueryEditorSQL(props: Props) {
    const queryWithDefaults: OpenObserveQuery = {
        format: undefined, // this property actually not used
        editorMode: EditorMode.Code, // set the code editor mode as default
        ...props.query,
    };

    const onQueryTypeChange = (option: ComboboxOption<string>) => {
        props.datasource.setQueryType(option.value);
        props.onChange({ ...queryWithDefaults, queryType: option.value });
        props.onRunQuery();
    };

    const onEnableSSEChange = (enableSSE: boolean) => {
        props.datasource.setEnableSSE(enableSSE);
        props.onChange({ ...queryWithDefaults, enableSSE: enableSSE });
        props.onRunQuery();
    }


    return <div>
        <Stack direction="row">
            <InlineField label="streamType" tooltip="stream type">
                <Combobox
                    id="query-editor-streamType"
                    value={props.query.queryType || 'logs'}
                    onChange={onQueryTypeChange}
                    options={[
                        { label: 'logs', value: 'logs' },
                        { label: 'metrics', value: 'metrics' },
                        { label: 'traces', value: 'traces' },
                    ]}
                />
            </InlineField>
            <InlineField label="enableSSE" tooltip="Enable Server-Sent Events (SSE) for real-time data streaming" grow>
                <InlineSwitch
                    value={props.query.enableSSE ?? true}
                    onChange={e => onEnableSSEChange(e.currentTarget.checked)}
                />
            </InlineField>
        </Stack>

        <SqlQueryEditor
            {...props}
            query={queryWithDefaults}
            datasource={props.datasource as unknown as SqlDatasource}
        />
    </div >
}
