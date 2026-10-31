import { DataSourcePlugin } from '@grafana/data';

import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditorSQL } from './components/QueryEditor';
import { OpenObserveDataSource } from './datasource';
import { OpenObserveOptions, OpenObserveQuery } from './types';

export const plugin = new DataSourcePlugin<OpenObserveDataSource, OpenObserveQuery, OpenObserveOptions>(
  OpenObserveDataSource
)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditorSQL);
