import { extractFirstTable, retargetStream } from './completion';

describe('extractFirstTable', () => {
  it('reads a double quoted stream name', () => {
    expect(extractFirstTable('SELECT * FROM "vnet_bff_dev" LIMIT 10')).toBe('vnet_bff_dev');
  });

  it('reads a backtick quoted stream name', () => {
    expect(extractFirstTable('select * from `my_stream`')).toBe('my_stream');
  });

  it('reads a bare identifier', () => {
    expect(extractFirstTable('SELECT body FROM logs')).toBe('logs');
  });

  it('accepts a dotted identifier', () => {
    expect(extractFirstTable('SELECT * FROM default.app_logs')).toBe('default.app_logs');
  });

  it('ignores the case of the keywords', () => {
    expect(extractFirstTable('SeLeCt * FrOm "x"')).toBe('x');
  });

  it('returns undefined without a FROM clause', () => {
    expect(extractFirstTable('SELECT 1')).toBeUndefined();
    expect(extractFirstTable('')).toBeUndefined();
  });

  it('does not match a column called from', () => {
    // "from" only counts as a keyword when it is a standalone word.
    expect(extractFirstTable('SELECT from_address FROM "t"')).toBe('t');
  });
});

describe('retargetStream', () => {
  it('replaces only the FROM target and keeps the rest byte for byte', () => {
    const sql = `SELECT service_name, count(*) AS "c" FROM "old_stream" WHERE severity = 'ERROR' GROUP BY service_name ORDER BY "c" DESC LIMIT 25`;
    expect(retargetStream(sql, 'new_stream')).toBe(
      `SELECT service_name, count(*) AS "c" FROM "new_stream" WHERE severity = 'ERROR' GROUP BY service_name ORDER BY "c" DESC LIMIT 25`
    );
  });

  it('keeps a backtick-quoted target and swaps the quoting style', () => {
    expect(retargetStream('SELECT * FROM `logs` LIMIT 5', 'app')).toBe('SELECT * FROM "app" LIMIT 5');
  });

  it('keeps a bare identifier target quoted', () => {
    expect(retargetStream('SELECT * FROM logs LIMIT 5', 'app')).toBe('SELECT * FROM "app" LIMIT 5');
  });

  it('leaves a LIMIT, WHERE and ORDER BY untouched', () => {
    const sql = 'select * from logs where a = 1 order by ts desc limit 10';
    expect(retargetStream(sql, 'app')).toBe('select * from "app" where a = 1 order by ts desc limit 10');
  });

  it('only rewrites the first FROM, not a column that happens to be named from', () => {
    const sql = 'SELECT from_address FROM "logs" WHERE from_address IS NOT NULL';
    expect(retargetStream(sql, 'app')).toBe('SELECT from_address FROM "app" WHERE from_address IS NOT NULL');
  });

  it('escapes a quote in the new stream name', () => {
    expect(retargetStream('SELECT * FROM "a"', 'we"ird')).toBe('SELECT * FROM "we""ird"');
  });

  it('seeds a statement when the editor is empty', () => {
    expect(retargetStream('', 'app')).toBe('SELECT * FROM "app" LIMIT 100');
    expect(retargetStream('   ', 'app')).toBe('SELECT * FROM "app" LIMIT 100');
  });

  it('appends FROM when the statement has none yet', () => {
    expect(retargetStream('SELECT 1', 'app')).toBe('SELECT 1 FROM "app"');
  });

  it('does not touch the projection when the stream is picked', () => {
    const sql = 'SELECT histogram(_timestamp, \'1 minute\') AS ts, avg(value) AS v FROM "metrics_a" GROUP BY ts';
    const out = retargetStream(sql, 'metrics_b');
    expect(out.startsWith("SELECT histogram(_timestamp, '1 minute') AS ts, avg(value) AS v FROM \"metrics_b\"")).toBe(true);
    expect(out.endsWith('GROUP BY ts')).toBe(true);
  });
});
