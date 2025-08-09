import React, { SyntheticEvent } from 'react';
import { Field, Input, SecretInput } from '@grafana/ui';
import {
    DataSourcePluginOptionsEditorProps,
    onUpdateDatasourceJsonDataOption,
    onUpdateDatasourceSecureJsonDataOption,
    updateDatasourcePluginResetOption,
} from '@grafana/data';
import { OpenObserveOptions, OpenObserveSecureJsonData } from '../types';
import { ConfigSection, DataSourceDescription } from '@grafana/plugin-ui';

interface Props extends DataSourcePluginOptionsEditorProps<OpenObserveOptions, OpenObserveSecureJsonData> { }

export function ConfigEditorSql(props: Props) {
    const { onOptionsChange, options } = props;
    const ELEMENT_WIDTH = 40;

    // BUG: when delete "url" value and save, it will reset to the previous value??
    const onDSOptionChanged = (property: keyof OpenObserveOptions) => {
        return (event: SyntheticEvent<HTMLInputElement>) => {
            onOptionsChange({ ...options, ...{ [property]: event.currentTarget.value } });
        };
    };

    const onResetPassWord = () => {
        updateDatasourcePluginResetOption(props, 'password');
    };

    return (
        <>
            <DataSourceDescription
                dataSourceName="OpenObserve"
                docsLink=""
                hasRequiredFields={true}
            />

            <hr />

            <ConfigSection title="Connection">
                <Field label="Host URL" required>
                    <Input
                        width={ELEMENT_WIDTH}
                        placeholder="http://localhost:8080"
                        value={options.url || ''}
                        onChange={onDSOptionChanged('url')}
                    />
                </Field>

                <Field label="Organization" required>
                    <Input
                        width={ELEMENT_WIDTH}
                        placeholder="openobserve organization name"
                        value={options.jsonData.database || ''} // mapping to 'database' in sqlEditor
                        onChange={onUpdateDatasourceJsonDataOption(props, 'database')}
                    />
                </Field>
            </ConfigSection>

            <hr />

            <ConfigSection title="Authentication">
                <Field label="Username" required>
                    <Input
                        width={ELEMENT_WIDTH}
                        placeholder="username"
                        value={options.user || ''}
                        onChange={onDSOptionChanged('user')}
                    />
                </Field>

                <Field label="PassWord" >
                    <SecretInput
                        width={ELEMENT_WIDTH}
                        placeholder="********"
                        isConfigured={options.secureJsonFields && options.secureJsonFields.password}
                        onReset={onResetPassWord}
                        onBlur={onUpdateDatasourceSecureJsonDataOption(props, 'password')}
                    />
                </Field>
            </ConfigSection>
        </>
    );
}
