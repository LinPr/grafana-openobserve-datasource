import React, { SyntheticEvent } from 'react';
import {
  DataSourcePluginOptionsEditorProps,
  onUpdateDatasourceJsonDataOption,
  onUpdateDatasourceJsonDataOptionChecked,
  onUpdateDatasourceSecureJsonDataOption,
  type SelectableValue,
  updateDatasourcePluginResetOption,
} from '@grafana/data';
import { ConfigSection, DataSourceDescription } from '@grafana/plugin-ui';
import { Field, Input, RadioButtonGroup, SecretInput, Switch } from '@grafana/ui';

import { OpenObserveOptions, OpenObserveSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<OpenObserveOptions, OpenObserveSecureJsonData> {}

const ELEMENT_WIDTH = 40;

type AuthMethod = 'basic' | 'token';

const AUTH_METHODS: Array<SelectableValue<AuthMethod>> = [
  { label: 'Basic auth', value: 'basic', description: 'Username and password, like the OpenObserve UI login' },
  { label: 'Token', value: 'token', description: 'An API or session token sent in the Authorization header' },
];

/**
 * Datasource configuration editor.
 *
 * Groups the settings by concern: connection, authentication, query defaults
 * and column mapping. Every field maps 1:1 to a backend jsonData key.
 */
export function ConfigEditor(props: Props): React.JSX.Element {
  const { onOptionsChange, options } = props;
  const jsonData = options.jsonData ?? {};

  const onOptionChanged = (property: keyof OpenObserveOptions) => {
    return (event: SyntheticEvent<HTMLInputElement>) => {
      onOptionsChange({ ...options, [property]: event.currentTarget.value });
    };
  };

  const onResetPassword = () => {
    updateDatasourcePluginResetOption(props, 'password');
  };

  const onResetToken = () => {
    updateDatasourcePluginResetOption(props, 'token');
  };

  const authMethod: AuthMethod = jsonData.authMethod === 'token' ? 'token' : 'basic';
  const organization = jsonData.organization ?? jsonData.database ?? 'default';

  return (
    <>
      <DataSourceDescription dataSourceName="OpenObserve" docsLink="" hasRequiredFields />

      <hr />

      <ConfigSection title="Connection">
        <Field
          label="URL"
          required
          description="Base URL of the OpenObserve server, e.g. http://localhost:5080"
          invalid={!options.url}
        >
          <Input
            width={ELEMENT_WIDTH}
            placeholder="http://localhost:5080"
            value={options.url ?? ''}
            onChange={onOptionChanged('url')}
          />
        </Field>

        <Field
          label="Organization"
          required
          description="The OpenObserve organization (first path segment of the API URL)"
        >
          <Input
            width={ELEMENT_WIDTH}
            placeholder="default"
            value={organization}
            onChange={onUpdateDatasourceJsonDataOption(props, 'organization')}
          />
        </Field>

        <Field label="Timeout (seconds)" description="Upper bound for a single query">
          <Input
            width={ELEMENT_WIDTH}
            type="number"
            min={1}
            placeholder="60"
            value={jsonData.timeoutSeconds ?? 60}
            onChange={onUpdateDatasourceJsonDataOption(props, 'timeoutSeconds')}
          />
        </Field>

        <Field label="Skip TLS verification" description="Accept self-signed certificates">
          <Switch
            value={jsonData.tlsSkipVerify ?? false}
            onChange={onUpdateDatasourceJsonDataOptionChecked(props, 'tlsSkipVerify')}
          />
        </Field>
      </ConfigSection>

      <hr />

      <ConfigSection title="Authentication">
        <Field label="Method">
          <RadioButtonGroup<AuthMethod>
            options={AUTH_METHODS}
            value={authMethod}
            onChange={value =>
              onOptionsChange({ ...options, jsonData: { ...jsonData, authMethod: value } })
            }
          />
        </Field>

        {authMethod === 'basic' ? (
          <>
            <Field label="User" required>
              <Input
                width={ELEMENT_WIDTH}
                placeholder="root@example.com"
                value={options.user ?? ''}
                onChange={onOptionChanged('user')}
              />
            </Field>

            <Field label="Password">
              <SecretInput
                width={ELEMENT_WIDTH}
                placeholder="********"
                isConfigured={Boolean(options.secureJsonFields?.password)}
                onReset={onResetPassword}
                onBlur={onUpdateDatasourceSecureJsonDataOption(props, 'password')}
              />
            </Field>
          </>
        ) : (
          <Field
            label="Token"
            description='Sent verbatim in the Authorization header, e.g. "Basic <base64>" or "Bearer <jwt>"'
          >
            <SecretInput
              width={ELEMENT_WIDTH}
              placeholder="Basic cm9vdEBleGFtcGxlLmNvbTp..."
              isConfigured={Boolean(options.secureJsonFields?.token)}
              onReset={onResetToken}
              onBlur={onUpdateDatasourceSecureJsonDataOption(props, 'token')}
            />
          </Field>
        )}
      </ConfigSection>

      <hr />

      <ConfigSection title="Query defaults">
        <Field label="Default limit" description="Row count used when a query has no LIMIT">
          <Input
            width={ELEMENT_WIDTH}
            type="number"
            min={1}
            placeholder="200"
            value={jsonData.defaultLimit ?? 200}
            onChange={onUpdateDatasourceJsonDataOption(props, 'defaultLimit')}
          />
        </Field>

        <Field label="Max limit" description="Hard cap on rows requested by any panel">
          <Input
            width={ELEMENT_WIDTH}
            type="number"
            min={1}
            placeholder="10000"
            value={jsonData.maxLimit ?? 10000}
            onChange={onUpdateDatasourceJsonDataOption(props, 'maxLimit')}
          />
        </Field>

        <Field label="Enable PromQL" description="Offer the Prometheus compatible query mode for metrics">
          <Switch
            value={jsonData.enablePrometheus ?? true}
            onChange={onUpdateDatasourceJsonDataOptionChecked(props, 'enablePrometheus')}
          />
        </Field>
      </ConfigSection>

      <hr />

      <ConfigSection title="Column mapping" description="Override the automatically detected columns">
        <Field label="Time field" description="Leave empty to auto-detect (_timestamp first)">
          <Input
            width={ELEMENT_WIDTH}
            placeholder="_timestamp"
            value={jsonData.timeField ?? ''}
            onChange={onUpdateDatasourceJsonDataOption(props, 'timeField')}
          />
        </Field>

        <Field label="Log message field" description="Leave empty to auto-detect (body, message, msg, ...)">
          <Input
            width={ELEMENT_WIDTH}
            placeholder="body"
            value={jsonData.logMessageField ?? ''}
            onChange={onUpdateDatasourceJsonDataOption(props, 'logMessageField')}
          />
        </Field>

        <Field label="Log level field" description="Leave empty to auto-detect (level, severity, ...)">
          <Input
            width={ELEMENT_WIDTH}
            placeholder="severity"
            value={jsonData.logLevelField ?? ''}
            onChange={onUpdateDatasourceJsonDataOption(props, 'logLevelField')}
          />
        </Field>
      </ConfigSection>
    </>
  );
}

/** Exported for the e2e helpers. */
export const AUTH_METHOD_OPTIONS = AUTH_METHODS;
