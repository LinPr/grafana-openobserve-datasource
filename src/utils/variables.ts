import { ScopedVars } from '@grafana/data';
import { getTemplateSrv } from '@grafana/runtime';

export const replace = (value?: string, scopedVars?: ScopedVars) => {
  if (value !== undefined) {
    return getTemplateSrv().replace(value, scopedVars, format);
  }
  return value;
};

export const format = (value: any) => {
  if (Array.isArray(value)) {
    if (value.some(v => typeof v === 'number')) {
      return `${value.join(",")}`;
    } else {
      return `${value.join("','")}`;
    }
  }
  return `${value}`;
  // return value
};
