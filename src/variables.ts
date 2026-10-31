import { CustomVariableSupport, DataQueryRequest, DataQueryResponse } from '@grafana/data';
import { Observable } from 'rxjs';

import { OpenObserveDataSource } from './datasource';
import { VariableQueryEditor } from './components/VariableQueryEditor';
import { OpenObserveQuery } from './types';

/**
 * Template variable support: an OpenObserve variable is a SQL query returning
 * one column, so it runs through the regular query path with a table format.
 */
export class OpenObserveVariableSupport extends CustomVariableSupport<
  OpenObserveDataSource,
  OpenObserveQuery,
  OpenObserveQuery
> {
  datasource: OpenObserveDataSource;
  editor = VariableQueryEditor;

  constructor(datasource: OpenObserveDataSource) {
    super();
    this.datasource = datasource;
  }

  query(request: DataQueryRequest<OpenObserveQuery>): Observable<DataQueryResponse> {
    const targets = (request.targets ?? []).map((query, index) => ({
      ...query,
      refId: query.refId || `tempVar${index}`,
      // Variables always want plain rows, never a log or graph frame.
      format: 'table' as const,
      queryMode: query.queryMode ?? 'sql',
    }));
    return this.datasource.query({ ...request, targets });
  }
}
