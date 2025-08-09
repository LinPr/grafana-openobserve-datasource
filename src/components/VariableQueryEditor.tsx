import React, { useCallback } from 'react';
import { SQLEditor } from '@grafana/plugin-ui';
import { OpenObserveDataSource } from 'datasource';
import { OpenObserveOptions, OpenObserveQuery } from 'types';
import { QueryEditorProps } from '@grafana/data';

type Props = QueryEditorProps<OpenObserveDataSource, OpenObserveQuery, OpenObserveOptions>;

export function VariableQueryEditorSql({ datasource, onChange, query, onRunQuery }: Props) {

    const onQueryChange = useCallback(
        (rawSql: string) => onChange({ ...query, rawQuery: true, rawSql }),
        [onChange, query]
    );

    return (<div>
        <SQLEditor
            query={query.rawSql!}
            onChange={onQueryChange}
            language={{
                id: 'sql',
                completionProvider: datasource.getSqlCompletionProvider(datasource.db)
            }}
        />
    </div>
    );

}
