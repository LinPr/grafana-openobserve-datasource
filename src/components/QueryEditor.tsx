import React from 'react';
import { QueryEditorProps } from '@grafana/data';

import { OpenObserveDataSource } from '../datasource';
import { OpenObserveOptions, OpenObserveQuery } from '../types';
import { EditorMode, SqlDatasource, SqlQueryEditor } from '@grafana/plugin-ui';
import { Combobox, InlineField, ComboboxOption } from '@grafana/ui';

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


    return <div>
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
        <SqlQueryEditor
            {...props}
            query={queryWithDefaults}
            datasource={props.datasource as unknown as SqlDatasource}
        />
    </div>
}
