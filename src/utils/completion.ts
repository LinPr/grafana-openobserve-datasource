import {
  ColumnDefinition,
  getStandardSQLCompletionProvider,
  LanguageCompletionProvider,
  TableDefinition,
  TableIdentifier,
  DB,
} from '@grafana/plugin-ui';
import { OpenObserveQuery } from 'types';

interface CompletionProviderGetterArgs {
  getColumns: React.MutableRefObject<(t: OpenObserveQuery) => Promise<ColumnDefinition[]>>;
  getTables: React.MutableRefObject<(d?: string) => Promise<TableDefinition[]>>;
}

export const getCompletionProvider = ({
  getColumns,
  getTables,
}: CompletionProviderGetterArgs): LanguageCompletionProvider => {
  return (monaco, language) => {
    return {
      ...(language && getStandardSQLCompletionProvider(monaco, language)),
      tables: {
        resolve: async () => {
          return await getTables.current();
        },
      },
      columns: {
        resolve: async (t?: TableIdentifier) => {
          return await getColumns.current({ table: t?.table, refId: 'A' });
        },
      },
    };
  };
};

export const completionFetchColumns = async (db: DB, q: OpenObserveQuery) => {
  const cols = await db.fields(q);
  if (cols.length > 0) {
    return cols.map((c) => {
      return { name: c.value, type: c.value, description: c.value };
    });
  } else {
    return [];
  }
};

export const completionFetchTables = async (db: DB) => {
  const tables = await db.tables();
  if (tables.length > 0) {
    return tables.map((t) => {
      return { name: t, completion: t };
    });
  } else {
    return [];
  }
};
