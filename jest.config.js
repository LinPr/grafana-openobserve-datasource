// force timezone to UTC to allow tests to work regardless of local timezone
// generally used by snapshots, but can affect specific tests
process.env.TZ = 'UTC';

const base = require('./.config/jest.config');

// The Go module cache and the local Grafana runtime both live inside the
// repository and contain thousands of foreign package.json files; keep Jest
// out of them so it does not report haste collisions or try to run their tests.
//
// NOTE: these are regular expressions and must not use `<rootDir>` — this
// checkout lives under a path containing `C++`, so the substituted absolute
// path is an invalid pattern ("Nothing to repeat").
const ignoredPaths = ['/\\.gocache/', '/\\.runtime/', '/\\bdist/'];

module.exports = {
  // Jest configuration provided by Grafana scaffolding
  ...base,

  modulePathIgnorePatterns: [...(base.modulePathIgnorePatterns ?? []), ...ignoredPaths],
  testPathIgnorePatterns: [...(base.testPathIgnorePatterns ?? []), '/node_modules/', ...ignoredPaths],
  watchPathIgnorePatterns: ignoredPaths,
};
