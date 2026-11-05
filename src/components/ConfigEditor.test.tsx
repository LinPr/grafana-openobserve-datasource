import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';

import { ConfigEditor } from './ConfigEditor';
import { OpenObserveOptions, OpenObserveSecureJsonData } from '../types';

/**
 * The config editor is a plain form: every field writes straight to
 * `options.jsonData` (or `options.url`, `options.user`) through
 * `onOptionsChange`. The tests assert the key each control writes to, which is
 * the contract the Go settings loader depends on.
 */
function renderConfig(jsonData: Partial<OpenObserveOptions> = {}, extra: Record<string, unknown> = {}) {
  const onOptionsChange = jest.fn();
  const options = {
    id: 1,
    uid: 'oo',
    orgId: 1,
    name: 'OpenObserve',
    type: 'linpr-openobserve-datasource',
    access: 'proxy',
    url: 'http://localhost:5080',
    user: '',
    basicAuth: false,
    isDefault: false,
    readOnly: false,
    withCredentials: false,
    jsonData,
    secureJsonFields: {},
    secureJsonData: {},
    ...extra,
  };

  const view = render(
    <ConfigEditor
      options={options as never}
      onOptionsChange={onOptionsChange}
      {...({} as Record<string, unknown>)}
    />
  );

  return { onOptionsChange, options, view };
}

const lastJsonData = (onOptionsChange: jest.Mock): Record<string, unknown> =>
  onOptionsChange.mock.calls[onOptionsChange.mock.calls.length - 1][0].jsonData;

const lastOptions = (onOptionsChange: jest.Mock): Record<string, unknown> =>
  onOptionsChange.mock.calls[onOptionsChange.mock.calls.length - 1][0];

describe('ConfigEditor', () => {
  afterEach(() => jest.clearAllMocks());

  it('writes the URL to the top-level option', () => {
    const { onOptionsChange } = renderConfig();
    fireEvent.change(screen.getByPlaceholderText('http://localhost:5080'), {
      target: { value: 'https://obs.example.com' },
    });
    expect(lastOptions(onOptionsChange).url).toBe('https://obs.example.com');
  });

  it('writes the organization into jsonData', () => {
    const { onOptionsChange } = renderConfig({});
    fireEvent.change(screen.getByPlaceholderText('default'), { target: { value: 'my-org' } });
    expect(lastJsonData(onOptionsChange).organization).toBe('my-org');
  });

  it('falls back to the legacy database key when organization is absent', () => {
    renderConfig({ database: 'legacy-org' });
    // The field shows the legacy value so a stored datasource keeps working.
    expect(screen.getByPlaceholderText('default')).toHaveValue('legacy-org');
  });

  it('prefers organization over the legacy key', () => {
    renderConfig({ organization: 'new-org', database: 'legacy-org' });
    expect(screen.getByPlaceholderText('default')).toHaveValue('new-org');
  });

  it('writes the timeout, default limit and max limit as jsonData keys', () => {
    const { onOptionsChange } = renderConfig();

    fireEvent.change(screen.getByPlaceholderText('60'), { target: { value: '120' } });
    expect(lastJsonData(onOptionsChange).timeoutSeconds).toBe('120');

    fireEvent.change(screen.getByPlaceholderText('200'), { target: { value: '300' } });
    expect(lastJsonData(onOptionsChange).defaultLimit).toBe('300');

    fireEvent.change(screen.getByPlaceholderText('10000'), { target: { value: '5000' } });
    expect(lastJsonData(onOptionsChange).maxLimit).toBe('5000');
  });

  it('toggles TLS skip verification', () => {
    const { onOptionsChange } = renderConfig({ tlsSkipVerify: false });
    // @grafana/ui Switch renders role="switch", not role="checkbox".
    const [tls] = screen.getAllByRole('switch');
    fireEvent.click(tls);
    expect(lastJsonData(onOptionsChange).tlsSkipVerify).toBe(true);
  });

  it('reflects the stored TLS setting', () => {
    renderConfig({ tlsSkipVerify: true });
    const [tls] = screen.getAllByRole('switch');
    expect(tls).toBeChecked();
  });

  it('toggles PromQL', () => {
    const { onOptionsChange } = renderConfig({ enablePrometheus: true });
    const [, promql] = screen.getAllByRole('switch');
    expect(promql).toBeChecked();
    fireEvent.click(promql);
    expect(lastJsonData(onOptionsChange).enablePrometheus).toBe(false);
  });

  it('defaults PromQL to enabled when the setting is absent', () => {
    renderConfig({});
    const [, promql] = screen.getAllByRole('switch');
    expect(promql).toBeChecked();
  });

  it('writes the column mapping overrides', () => {
    const { onOptionsChange } = renderConfig();

    fireEvent.change(screen.getByPlaceholderText('_timestamp'), { target: { value: 'ts' } });
    expect(lastJsonData(onOptionsChange).timeField).toBe('ts');

    fireEvent.change(screen.getByPlaceholderText('body'), { target: { value: 'msg' } });
    expect(lastJsonData(onOptionsChange).logMessageField).toBe('msg');

    fireEvent.change(screen.getByPlaceholderText('severity'), { target: { value: 'lvl' } });
    expect(lastJsonData(onOptionsChange).logLevelField).toBe('lvl');
  });

  it('shows the password field for basic auth', () => {
    const { view } = renderConfig({ authMethod: 'basic' });
    expect(screen.getByPlaceholderText('root@example.com')).toBeInTheDocument();
    expect(view.container.querySelector('input[type="password"], input[placeholder="********"]')).not.toBeNull();
    expect(screen.queryByPlaceholderText(/^Basic /)).not.toBeInTheDocument();
  });

  it('shows the token field for token auth', () => {
    renderConfig({ authMethod: 'token' });
    expect(screen.queryByPlaceholderText('root@example.com')).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText(/Basic cm9vd/)).toBeInTheDocument();
  });

  it('writes the auth method when switching to token', () => {
    const { onOptionsChange } = renderConfig({ authMethod: 'basic' });
    fireEvent.click(screen.getByText('Token'));
    expect(lastJsonData(onOptionsChange).authMethod).toBe('token');
  });

  it('marks a configured secret so the reset button appears', () => {
    renderConfig({ authMethod: 'basic' }, { secureJsonFields: { password: true } });
    expect(screen.getByText('Reset')).toBeInTheDocument();
  });
});

describe('ConfigEditor secret data', () => {
  it('exposes the secure json data type', () => {
    const payload: OpenObserveSecureJsonData = { token: 'abc' };
    expect(payload.token).toBe('abc');
  });
});
