// import { DataFrame, DataFrameView, DataSourceInstanceSettings, ScopedVars, TimeRange } from '@grafana/data';
import { CoreApp, DataSourceInstanceSettings, ScopedVars, AdHocVariableFilter, DataSourceGetTagKeysOptions, GetTagResponse, MetricFindValue, DataSourceGetTagValuesOptions } from '@grafana/data';
import {
    // BackendDataSourceResponse,
    DataSourceWithBackend,
    // getTemplateSrv,
    // FetchResponse,
    // getBackendSrv,
    // toDataQueryResponse,
} from '@grafana/runtime';
// import { DB, QueryFormat, SQLSelectableValue, ValidationResults, LanguageCompletionProvider } from '@grafana/plugin-ui';
import { DB, SQLSelectableValue, ValidationResults, LanguageCompletionProvider } from '@grafana/plugin-ui';
import { OpenObserveQuery, OpenObserveOptions, ListStreamResponse, DEFAULT_QUERY } from 'types';
// import { buildColumnQuery, buildTableQuery } from './utils/queries';
import { completionFetchColumns, completionFetchTables, getCompletionProvider } from './utils/completion';
import { AGGREGATE_FNS } from './utils/constants';
import { OpenObserveVariableSupport } from 'variables';
import { replace } from 'utils/variables';

export class OpenObserveDataSource extends DataSourceWithBackend<OpenObserveQuery, OpenObserveOptions> {
    annotations = {};
    db: DB;
    dataset: string;
    queryType: string;
    completionProvider: LanguageCompletionProvider | undefined;

    constructor(private instanceSettings: DataSourceInstanceSettings<OpenObserveOptions>) {
        super(instanceSettings);
        this.db = this.getDB();
        this.dataset = this.instanceSettings.jsonData.database;
        this.variables = new OpenObserveVariableSupport(this);
        this.queryType = "";
    }

    setQueryType(queryType: string) {
        this.queryType = queryType;
    }

    /**
     * getDefaultQuery set the default query for the OpenObserve datasource.
     */
    getDefaultQuery(app: CoreApp): Partial<OpenObserveQuery> {
        return DEFAULT_QUERY
    }

    /**
     * Applies template variables to the given OpenObserveQuery object.
     * Replaces any occurrences of template variables in the raw SQL string with their corresponding values.
     */
    applyTemplateVariables(query: OpenObserveQuery, scopedVars: ScopedVars, filters?: AdHocVariableFilter[]): OpenObserveQuery {
        return {
            ...query,
            rawSql: replace(query.rawSql ?? '', scopedVars) ?? '',
            adhocFilters: filters ?? [],
            // queryType: getTemplateSrv().replace(query.queryType, scopedVars),
        };
    }

    getTagKeys(options?: DataSourceGetTagKeysOptions<OpenObserveQuery> | undefined): Promise<GetTagResponse> | Promise<MetricFindValue[]> {
        // This method is not implemented in the original code, but it can be used to fetch tag keys if needed.
        // For now, we return an empty array as a placeholder.
        return Promise.resolve([]);
    }

    getTagValues(options: DataSourceGetTagValuesOptions<OpenObserveQuery>): Promise<GetTagResponse> | Promise<MetricFindValue[]> {
        // This method is not implemented in the original code, but it can be used to fetch tag values if needed.
        // For now, we return an empty array as a placeholder.
        return Promise.resolve([]);
    }


    /**
     * Filters the query based on the provided OpenObserveQuery object.
     * Returns true if the query contains raw SQL, false otherwise.
     */
    filterQuery(query: OpenObserveQuery): boolean {
        return Boolean(query.rawSql);
    }

    /**
     * Validates a OpenObserve query.
     * Returns a ValidationResults object.
     */
    validateQuery(query: OpenObserveQuery): ValidationResults {
        return { query, isError: false, isValid: true, error: '' };
    }

    /**
     * listStreamInfo used for creating dynamic variables
     */
    async listStreamInfo(streamType: string): Promise<ListStreamResponse> {
        const params = {
            organization: this.dataset,
            type: streamType, // Specify the type of stream, e.g., 'logs',
        };

        return await this.getResource(`/openobserve/streams`, params);
    }


    /**
     * fetchTables used for smart completion in the query editor, works in coder and builder mode
     */
    async fetchTables(): Promise<string[]> {
        const listStreamInfoResp = await this.listStreamInfo(this.queryType);

        const tableNames: string[] = [];
        for (const [key] of Object.entries(listStreamInfoResp)) {
            tableNames.push(key);
        }

        tableNames.sort();
        return tableNames;
    }

    /**
     * fetchFields used for smart completion in the query editor, works in coder and builder mode
     */
    async fetchFields(query: Partial<OpenObserveQuery>): Promise<SQLSelectableValue[]> {

        const listStreamInfoResp = await this.listStreamInfo(query.queryType ?? "");
        const selectedColumns: SQLSelectableValue[] = [];

        for (const [streamKey, streamColumns] of Object.entries(listStreamInfoResp)) {
            if (streamKey === query.table) {
                if (Array.isArray(streamColumns)) {
                    for (const columnName of streamColumns) {
                        // avoid duplicatesfield    
                        if (!selectedColumns.some(f => f.name === columnName)) {
                            selectedColumns.push({ name: columnName, text: columnName, value: columnName, type: 'string', label: columnName });
                        }
                    }
                }
            }
        }
        selectedColumns.sort();
        return selectedColumns;
    }

    /**
     * Returns the DB object for the datasource.
     * The DB object provides methods for interacting with the datasource.
     */
    getDB(): DB {
        if (this.db !== undefined) {
            return this.db;
        }
        return {
            dsID: () => this.id,
            lookup: () => Promise.resolve([]),
            datasets: () => Promise.resolve([]),
            functions: async () => Promise.resolve(AGGREGATE_FNS),
            tables: async () => await this.fetchTables(),
            fields: async (query: OpenObserveQuery) => await this.fetchFields(query),
            validateQuery: async (query: OpenObserveQuery) => this.validateQuery(query),
            getSqlCompletionProvider: () => this.getSqlCompletionProvider(this.db),
            disableDatasets: true,
        };
    }

    /**
     * Retrieves the SQL completion provider associated with the data source.
     * If the provider is not already initialized, it will be created and returned.
     */
    getSqlCompletionProvider(db: DB): LanguageCompletionProvider {
        if (this.completionProvider !== undefined) {
            return this.completionProvider;
        }
        const args = {
            getColumns: { current: (query: OpenObserveQuery) => completionFetchColumns(db, query) },
            getTables: { current: () => completionFetchTables(db) },
        };
        this.completionProvider = getCompletionProvider(args);
        return this.completionProvider;
    }
}
