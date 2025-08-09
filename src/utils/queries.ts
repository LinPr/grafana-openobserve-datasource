export const buildTableQuery = () => {
  return `
    SELECT table_name as "tables"
    FROM INFORMATION_SCHEMA.TABLES
    WHERE table_type = 'BASE TABLE'
    AND table_schema NOT IN ('information_schema', 'pg_catalog');
  `;
};

export const buildColumnQuery = (table: string) => {
  return `
    SELECT column_name as "columns"
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE table_name = '${table}';
  `;
};
