import { CustomVariableSupport, DataQueryRequest, DataQueryResponse } from '@grafana/data';
import { VariableQueryEditorSql } from 'components/VariableQueryEditor';
import { OpenObserveDataSource } from 'datasource';
import { uniqueId } from 'lodash';
import { Observable } from 'rxjs';
import { OpenObserveQuery } from 'types';

export class OpenObserveVariableSupport extends CustomVariableSupport<OpenObserveDataSource> {
    datasource: OpenObserveDataSource;
    editor = VariableQueryEditorSql;

    constructor(datasource: OpenObserveDataSource) {
        super();
        this.datasource = datasource;
    }

    query(request: DataQueryRequest<OpenObserveQuery>): Observable<DataQueryResponse> {
        const queries = request.targets.map((query) => {
            return { ...query, refId: query.refId || uniqueId('tempVar') };
        });
        return this.datasource.query({ ...request, targets: queries });
    }
}
