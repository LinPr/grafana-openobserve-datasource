import { AdHocVariableFilter } from '@grafana/data';
import { SQLOptions, SQLQuery, EditorMode, QueryFormat } from '@grafana/plugin-ui';


/**
 * Represents a query specific to the OpenObserve data source.
 */
export interface OpenObserveQuery extends SQLQuery {
    format?: QueryFormat;
    editorMode?: EditorMode;
    // streamType?: string;
    adhocFilters?: AdHocVariableFilter[];
    enableSSE?: boolean;
}

export const DEFAULT_QUERY: Partial<OpenObserveQuery> = {
    queryType: "logs",
    enableSSE: true
};

/**
 * These are options configured for each DataSource instance
 */
export interface OpenObserveOptions extends SQLOptions {
    url: string
}

/**
 * Value that is used in the backend, but never sent over HTTP to the frontend
 */
export interface OpenObserveSecureJsonData {
    password?: string;
}

/**
 * get openobserve streamInfo
 */
export type ListStreamResponse = {}
