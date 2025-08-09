import { DataSourcePlugin } from '@grafana/data';
import { OpenObserveDataSource } from './datasource';
import { OpenObserveQuery, OpenObserveOptions } from './types';
import { QueryEditorSQL } from 'components/QueryEditor';
import { ConfigEditorSql } from 'components/ConfigEditor';

export const plugin = new DataSourcePlugin<OpenObserveDataSource, OpenObserveQuery, OpenObserveOptions>(OpenObserveDataSource)
  .setConfigEditor(ConfigEditorSql)
  .setQueryEditor(QueryEditorSQL);
