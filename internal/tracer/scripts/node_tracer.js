/**
 * Aperio Node.js Tracer — injected via node --require
 *
 * Captures:
 *   1. Function calls via Module.prototype.require wrapping
 *   2. Subprocess execution (child_process.exec/spawn/execSync/etc.)
 *   3. Filesystem I/O (fs.readFile, fs.writeFile, etc.)
 *   4. Network I/O (net.Socket.connect, http/https requests)
 *   5. Database queries (generic wrapping for common drivers)
 *
 * Environment variables:
 *   APERIO_TRACE_OUTPUT  - path to write trace JSON
 *   APERIO_TRACE_DIR     - project directory to trace (only files in this tree)
 *   APERIO_EXCLUDE       - comma-separated path substrings to exclude
 *   APERIO_MODE          - "record" (default) or "replay"
 *   APERIO_REPLAY_DATA   - path to replay data file (for replay mode)
 */

'use strict';

const Module = require('module');
const path = require('path');
const fs = require('fs');

const traceOutput = process.env.APERIO_TRACE_OUTPUT || '';
const traceDir = process.env.APERIO_TRACE_DIR || '';
const mode = process.env.APERIO_MODE || 'record';
const replayDataPath = process.env.APERIO_REPLAY_DATA || '';
const excludePatterns = (process.env.APERIO_EXCLUDE || '')
  .split(',')
  .filter(Boolean);

const defaultExcludes = ['node_modules', 'aperio_tracer', '.cache'];

const spans = [];
const callStack = [];
let counter = 0;

// Replay data
let replayData = {};
let replayCounters = {};

function truncate(s, maxLen = 2000) {
  if (s === null || s === undefined) return null;
  s = String(s);
  if (s.length > maxLen) return s.substring(0, maxLen) + `... (${s.length} bytes total)`;
  return s;
}

function nextId() {
  counter++;
  return `node-${counter}`;
}

function currentParentId() {
  return callStack.length > 0 ? callStack[callStack.length - 1] : '';
}

function addIOSpan(spanType, name, attributes, startTime, endTime) {
  spans.push({
    id: nextId(),
    parent_id: currentParentId(),
    type: spanType,
    name: name,
    start_time: (startTime || Date.now()) / 1000,
    end_time: (endTime || Date.now()) / 1000,
    attributes: attributes,
  });
}

// =============================================================================
// Command classification
// =============================================================================

const EXEC_CATEGORIES = {
  git: ['git'],
  filesystem: ['cd','mkdir','rmdir','rm','cp','mv','ls','find','chmod','chown','ln','touch','stat','realpath','readlink','basename','dirname','mktemp','install','tree','du','df'],
  editor: ['sed','awk','cat','head','tail','tee','grep','rg','sort','uniq','wc','cut','tr','xargs','diff','patch','jq','yq','less','more','vi','vim','nano','ed'],
  network: ['curl','wget','ssh','scp','sftp','nc','ncat','dig','nslookup','host','ping','traceroute','openssl','http'],
  package_manager: ['pip','pip3','npm','npx','yarn','pnpm','bun','cargo','apt','apt-get','brew','conda','mamba','gem','composer','poetry','pdm','uv','pipx'],
  build: ['make','cmake','ninja','gcc','g++','cc','clang','tsc','esbuild','vite','webpack','rollup','javac','rustc','bazel','gradle','mvn'],
  runtime: ['python','python3','node','java','ruby','deno','php','perl','bash','sh','zsh'],
  docker: ['docker','docker-compose','podman','kubectl','helm'],
  test: ['pytest','jest','vitest','mocha','phpunit','rspec','unittest'],
  lint: ['eslint','prettier','black','ruff','isort','mypy','pyright','flake8','pylint','golangci-lint','clippy','shellcheck','stylelint','biome'],
  env: ['export','env','printenv','source','which','where','echo','printf','set','unset'],
};

const EXEC_TWO_WORD = {
  'go get':'package_manager','go install':'package_manager','go mod':'package_manager',
  'go build':'build','cargo build':'build','docker build':'build',
  'go run':'runtime','cargo run':'runtime','bun run':'runtime',
  'go test':'test','cargo test':'test','npm test':'test','yarn test':'test',
};

const EXEC_LOOKUP = {};
for (const [cat, cmds] of Object.entries(EXEC_CATEGORIES)) {
  for (const cmd of cmds) EXEC_LOOKUP[cmd] = cat;
}

function classifyCommand(cmdStr) {
  const parts = cmdStr.trim().split(/\s+/);
  if (parts.length === 0) return { category: 'other', subcategory: '' };

  const exe = path.basename(parts[0]);
  if (parts.length >= 2) {
    const twoWord = exe + ' ' + parts[1];
    if (EXEC_TWO_WORD[twoWord]) {
      return { category: EXEC_TWO_WORD[twoWord], subcategory: parts[2] || '' };
    }
  }
  if (EXEC_LOOKUP[exe]) {
    return { category: EXEC_LOOKUP[exe], subcategory: parts[1] || '' };
  }
  return { category: 'other', subcategory: '' };
}

function getReplayResponse(spanType) {
  if (!replayData[spanType]) return null;
  const idx = replayCounters[spanType] || 0;
  replayCounters[spanType] = idx + 1;
  const entries = replayData[spanType];
  return idx < entries.length ? entries[idx] : null;
}

function shouldTrace(filename) {
  if (!filename) return false;
  for (const pattern of defaultExcludes) {
    if (filename.includes(pattern)) return false;
  }
  for (const pattern of excludePatterns) {
    if (filename.includes(pattern)) return false;
  }
  if (traceDir) {
    try {
      const realFile = fs.realpathSync(filename);
      const realDir = fs.realpathSync(traceDir);
      return realFile.startsWith(realDir);
    } catch {
      return false;
    }
  }
  return true;
}

// =============================================================================
// 1. Function wrapping (Module.prototype.require)
// =============================================================================

function wrapFunction(fn, name, filename) {
  if (typeof fn !== 'function' || fn.__aperio_wrapped) return fn;

  const wrapped = function (...args) {
    const spanId = nextId();
    const parentId = currentParentId();
    const startTime = Date.now();

    const argsInfo = {};
    try {
      args.forEach((arg, i) => {
        let repr = String(arg);
        if (repr.length > 200) repr = repr.substring(0, 200) + '...';
        argsInfo[`arg${i}`] = repr;
      });
    } catch {}

    const span = {
      id: spanId,
      parent_id: parentId,
      type: 'FUNCTION',
      name: name,
      start_time: startTime / 1000,
      end_time: 0,
      attributes: { module: name.split('.')[0] || '', function: name.split('.').pop() || name, filename, args: argsInfo },
    };

    const spanIndex = spans.length;
    spans.push(span);
    callStack.push(spanId);

    let result;
    try {
      result = fn.apply(this, args);
    } catch (err) {
      spans[spanIndex].end_time = Date.now() / 1000;
      if (callStack[callStack.length - 1] === spanId) callStack.pop();
      throw err;
    }

    if (result && typeof result.then === 'function') {
      return result.then(
        (val) => {
          spans[spanIndex].end_time = Date.now() / 1000;
          if (callStack[callStack.length - 1] === spanId) callStack.pop();
          return val;
        },
        (err) => {
          spans[spanIndex].end_time = Date.now() / 1000;
          if (callStack[callStack.length - 1] === spanId) callStack.pop();
          throw err;
        }
      );
    }

    spans[spanIndex].end_time = Date.now() / 1000;
    callStack.pop();
    return result;
  };

  wrapped.__aperio_wrapped = true;
  Object.defineProperty(wrapped, 'name', { value: fn.name });
  Object.defineProperty(wrapped, 'length', { value: fn.length });
  return wrapped;
}

function wrapExports(exports, moduleName, filename) {
  if (!exports || typeof exports !== 'object') return exports;
  for (const key of Object.keys(exports)) {
    try {
      if (typeof exports[key] === 'function' && !exports[key].__aperio_wrapped) {
        exports[key] = wrapFunction(exports[key], `${moduleName}.${key}`, filename);
      }
    } catch {}
  }
  return exports;
}

const originalRequire = Module.prototype.require;
Module.prototype.require = function (id) {
  const result = originalRequire.apply(this, arguments);
  try {
    const resolvedPath = Module._resolveFilename(id, this);
    if (shouldTrace(resolvedPath)) {
      const moduleName = path.basename(resolvedPath, path.extname(resolvedPath));
      return wrapExports(result, moduleName, resolvedPath);
    }
  } catch {}
  return result;
};

// =============================================================================
// 2. Subprocess execution (child_process)
// =============================================================================

function patchChildProcess() {
  try {
    const cp = require('child_process');

    const origExecSync = cp.execSync;
    cp.execSync = function (command, options) {
      const cmdStr = String(command);
      const cls = classifyCommand(cmdStr);

      if (mode === 'replay') {
        const replay = getReplayResponse('EXEC');
        if (replay) {
          addIOSpan('EXEC', `execSync: ${cmdStr.substring(0, 80)}`, {
            command: cmdStr, 'exec.category': cls.category, 'exec.subcategory': cls.subcategory,
            exit_code: replay.exit_code || 0, stdout: replay.stdout || '', source: 'replay',
          });
          return Buffer.from(replay.stdout || '');
        }
      }

      const start = Date.now();
      let result;
      try {
        result = origExecSync.call(this, command, options);
      } catch (err) {
        addIOSpan('EXEC', `execSync: ${cmdStr.substring(0, 80)}`, {
          command: cmdStr, 'exec.category': cls.category, 'exec.subcategory': cls.subcategory,
          exit_code: err.status || 1,
          stdout: truncate(err.stdout), stderr: truncate(err.stderr),
        }, start);
        throw err;
      }
      addIOSpan('EXEC', `execSync: ${cmdStr.substring(0, 80)}`, {
        command: cmdStr, 'exec.category': cls.category, 'exec.subcategory': cls.subcategory,
        exit_code: 0, stdout: truncate(result),
      }, start);
      return result;
    };

    const origExec = cp.exec;
    cp.exec = function (command, ...args) {
      const cmdStr = String(command);
      const cls = classifyCommand(cmdStr);
      const start = Date.now();

      const child = origExec.call(this, command, ...args);

      let stdout = '', stderr = '';
      if (child.stdout) child.stdout.on('data', (d) => { stdout += d; });
      if (child.stderr) child.stderr.on('data', (d) => { stderr += d; });

      child.on('close', (code) => {
        addIOSpan('EXEC', `exec: ${cmdStr.substring(0, 80)}`, {
          command: cmdStr, 'exec.category': cls.category, 'exec.subcategory': cls.subcategory,
          exit_code: code || 0, stdout: truncate(stdout), stderr: truncate(stderr),
        }, start);
      });

      return child;
    };

    const origSpawnSync = cp.spawnSync;
    cp.spawnSync = function (command, spawnArgs, options) {
      const cmdStr = [command, ...(spawnArgs || [])].join(' ');
      const cls = classifyCommand(cmdStr);
      const start = Date.now();
      const result = origSpawnSync.call(this, command, spawnArgs, options);
      addIOSpan('EXEC', `spawnSync: ${cmdStr.substring(0, 80)}`, {
        command: cmdStr, 'exec.category': cls.category, 'exec.subcategory': cls.subcategory,
        exit_code: result.status || 0,
        stdout: truncate(result.stdout), stderr: truncate(result.stderr),
      }, start);
      return result;
    };
  } catch {}
}

// =============================================================================
// 3. Filesystem I/O (fs module)
// =============================================================================

function patchFilesystem() {
  const origReadFileSync = fs.readFileSync;
  fs.readFileSync = function (filePath, options) {
    const pathStr = String(filePath);

    if (mode === 'replay') {
      const replay = getReplayResponse('FS_READ');
      if (replay && replay.path === pathStr) {
        addIOSpan('FS_READ', `readFileSync: ${pathStr}`, {
          path: pathStr, size: (replay.content || '').length, source: 'replay',
        });
        return options && (options === 'utf8' || options.encoding) ? replay.content : Buffer.from(replay.content || '');
      }
    }

    const start = Date.now();
    const result = origReadFileSync.call(this, filePath, options);
    addIOSpan('FS_READ', `readFileSync: ${pathStr}`, {
      path: pathStr, size: result.length,
      content: truncate(result),
    }, start);
    return result;
  };

  const origWriteFileSync = fs.writeFileSync;
  fs.writeFileSync = function (filePath, data, options) {
    const pathStr = String(filePath);
    if (pathStr === traceOutput) return origWriteFileSync.call(this, filePath, data, options);

    const start = Date.now();
    const result = origWriteFileSync.call(this, filePath, data, options);
    addIOSpan('FS_WRITE', `writeFileSync: ${pathStr}`, {
      path: pathStr, size: data.length,
      content: truncate(data),
    }, start);
    return result;
  };

  // Async variants
  const origReadFile = fs.readFile;
  fs.readFile = function (filePath, ...args) {
    const pathStr = String(filePath);
    const start = Date.now();
    const cb = typeof args[args.length - 1] === 'function' ? args.pop() : null;

    origReadFile.call(this, filePath, ...args, function (err, data) {
      if (!err) {
        addIOSpan('FS_READ', `readFile: ${pathStr}`, {
          path: pathStr, size: data.length,
        }, start);
      }
      if (cb) cb(err, data);
    });
  };

  const origWriteFile = fs.writeFile;
  fs.writeFile = function (filePath, data, ...args) {
    const pathStr = String(filePath);
    if (pathStr === traceOutput) return origWriteFile.call(this, filePath, data, ...args);

    const start = Date.now();
    const cb = typeof args[args.length - 1] === 'function' ? args.pop() : null;

    origWriteFile.call(this, filePath, data, ...args, function (err) {
      if (!err) {
        addIOSpan('FS_WRITE', `writeFile: ${pathStr}`, {
          path: pathStr, size: data.length,
        }, start);
      }
      if (cb) cb(err);
    });
  };
}

// =============================================================================
// 4. Network I/O (net/http/https)
// =============================================================================

function patchNetwork() {
  try {
    const net = require('net');
    const origConnect = net.Socket.prototype.connect;

    net.Socket.prototype.connect = function (...args) {
      let host = 'unknown', port = 0;
      if (args[0] && typeof args[0] === 'object') {
        host = args[0].host || args[0].path || 'unknown';
        port = args[0].port || 0;
      } else if (typeof args[0] === 'number') {
        port = args[0];
        host = args[1] || 'localhost';
      }

      addIOSpan('NET_IO', `connect: ${host}:${port}`, {
        operation: 'connect', host: String(host), port: port,
      });

      return origConnect.apply(this, args);
    };
  } catch {}

  // Patch http/https request to capture full request/response
  for (const proto of ['http', 'https']) {
    try {
      const mod = require(proto);
      const origRequest = mod.request;

      mod.request = function (urlOrOpts, ...args) {
        let urlStr = '';
        let method = 'GET';

        if (typeof urlOrOpts === 'string') {
          urlStr = urlOrOpts;
        } else if (urlOrOpts && typeof urlOrOpts === 'object') {
          if (urlOrOpts.href) urlStr = urlOrOpts.href;
          else urlStr = `${proto}://${urlOrOpts.hostname || urlOrOpts.host || 'unknown'}${urlOrOpts.path || '/'}`;
          method = urlOrOpts.method || 'GET';
        }

        const start = Date.now();
        const req = origRequest.call(this, urlOrOpts, ...args);

        req.on('response', (res) => {
          let body = '';
          res.on('data', (chunk) => { body += chunk; });
          res.on('end', () => {
            addIOSpan('NET_IO', `${method} ${urlStr.substring(0, 80)}`, {
              operation: 'http_request',
              method: method,
              url: urlStr,
              status_code: res.statusCode,
              response_size: body.length,
              response_preview: truncate(body, 500),
            }, start);
          });
        });

        return req;
      };
    } catch {}
  }
}

// =============================================================================
// 5. Database patching
// =============================================================================

function patchDatabases() {
  // Patch on require — when a DB driver is loaded, wrap its query methods
  const origRequire = Module.prototype.require;
  const dbPatchers = {
    'pg': patchPg,
    'mysql2': patchMysql,
    'mysql': patchMysql,
    'better-sqlite3': patchBetterSqlite3,
    'redis': patchRedis,
    'ioredis': patchIORedis,
    'mongodb': patchMongoDB,
  };

  Module.prototype.require = function (id) {
    const result = origRequire.apply(this, arguments);
    if (dbPatchers[id] && !result.__aperio_db_patched) {
      try {
        dbPatchers[id](result);
        result.__aperio_db_patched = true;
      } catch {}
    }
    // Also keep function wrapping for project modules
    try {
      const resolvedPath = Module._resolveFilename(id, this);
      if (shouldTrace(resolvedPath)) {
        const moduleName = path.basename(resolvedPath, path.extname(resolvedPath));
        return wrapExports(result, moduleName, resolvedPath);
      }
    } catch {}
    return result;
  };
}

function patchPg(pg) {
  if (!pg.Client || !pg.Client.prototype) return;
  const origQuery = pg.Client.prototype.query;
  pg.Client.prototype.query = function (queryText, ...args) {
    const sql = typeof queryText === 'string' ? queryText : (queryText.text || String(queryText));
    const start = Date.now();

    const result = origQuery.call(this, queryText, ...args);
    if (result && typeof result.then === 'function') {
      return result.then((res) => {
        addIOSpan('DB_QUERY', `pg: ${sql.substring(0, 80)}`, {
          database: 'postgresql', query: truncate(sql, 1000), row_count: res.rowCount,
        }, start);
        return res;
      });
    }
    return result;
  };
}

function patchMysql(mysql) {
  // mysql2 and mysql share a similar API
  if (!mysql.Connection || !mysql.Connection.prototype) return;
  const origQuery = mysql.Connection.prototype.query;
  mysql.Connection.prototype.query = function (sql, ...args) {
    const sqlStr = typeof sql === 'string' ? sql : String(sql);
    const start = Date.now();
    addIOSpan('DB_QUERY', `mysql: ${sqlStr.substring(0, 80)}`, {
      database: 'mysql', query: truncate(sqlStr, 1000),
    }, start);
    return origQuery.call(this, sql, ...args);
  };
}

function patchBetterSqlite3(Database) {
  if (typeof Database !== 'function') return;
  const origPrepare = Database.prototype.prepare;
  if (!origPrepare) return;

  Database.prototype.prepare = function (sql) {
    const stmt = origPrepare.call(this, sql);
    const origRun = stmt.run;
    const origGet = stmt.get;
    const origAll = stmt.all;

    stmt.run = function (...args) {
      const start = Date.now();
      const result = origRun.apply(this, args);
      addIOSpan('DB_QUERY', `sqlite: ${sql.substring(0, 80)}`, {
        database: 'sqlite', query: truncate(sql, 1000), operation: 'run',
      }, start);
      return result;
    };
    stmt.get = function (...args) {
      const start = Date.now();
      const result = origGet.apply(this, args);
      addIOSpan('DB_QUERY', `sqlite: ${sql.substring(0, 80)}`, {
        database: 'sqlite', query: truncate(sql, 1000), operation: 'get',
      }, start);
      return result;
    };
    stmt.all = function (...args) {
      const start = Date.now();
      const result = origAll.apply(this, args);
      addIOSpan('DB_QUERY', `sqlite: ${sql.substring(0, 80)}`, {
        database: 'sqlite', query: truncate(sql, 1000), operation: 'all', row_count: result.length,
      }, start);
      return result;
    };
    return stmt;
  };
}

function patchRedis(redis) {
  if (!redis.RedisClient || !redis.RedisClient.prototype) return;
  const origSendCommand = redis.RedisClient.prototype.send_command;
  if (!origSendCommand) return;

  redis.RedisClient.prototype.send_command = function (command, args, cb) {
    const cmdStr = [command, ...(args || []).slice(0, 3)].join(' ');
    addIOSpan('DB_QUERY', `redis: ${cmdStr.substring(0, 80)}`, {
      database: 'redis', command: cmdStr,
    });
    return origSendCommand.call(this, command, args, cb);
  };
}

function patchIORedis(Redis) {
  if (!Redis.prototype) return;
  const origSendCommand = Redis.prototype.sendCommand;
  if (!origSendCommand) return;

  Redis.prototype.sendCommand = function (command) {
    const cmdName = command.name || 'unknown';
    const cmdArgs = (command.args || []).slice(0, 3).join(' ');
    addIOSpan('DB_QUERY', `redis: ${cmdName} ${cmdArgs}`.substring(0, 80), {
      database: 'redis', command: `${cmdName} ${cmdArgs}`,
    });
    return origSendCommand.call(this, command);
  };
}

function patchMongoDB(mongodb) {
  // MongoDB driver 4.x+ uses Collection methods
  if (!mongodb.Collection || !mongodb.Collection.prototype) return;
  for (const method of ['find', 'insertOne', 'insertMany', 'updateOne', 'updateMany', 'deleteOne', 'deleteMany']) {
    const orig = mongodb.Collection.prototype[method];
    if (!orig) continue;

    mongodb.Collection.prototype[method] = function (...args) {
      addIOSpan('DB_QUERY', `mongo: ${method}`, {
        database: 'mongodb', operation: method,
        query: truncate(JSON.stringify(args[0] || {}), 1000),
      });
      return orig.apply(this, args);
    };
  }
}

// =============================================================================
// Output and install
// =============================================================================

function epochToISO(ts) {
  return new Date(ts * 1000).toISOString();
}

function flushSpans() {
  if (!traceOutput) return;

  const data = spans.map((s) => ({
    ...s,
    start_time: s.start_time ? epochToISO(s.start_time) : null,
    end_time: s.end_time ? epochToISO(s.end_time) : null,
  }));

  try {
    // Use the original fs.writeFileSync to avoid triggering our patch
    const origWrite = fs.__aperio_origWriteFileSync || fs.writeFileSync;
    origWrite(traceOutput, JSON.stringify(data, null, 2));
  } catch (err) {
    process.stderr.write(`[aperio] Error writing trace: ${err.message}\n`);
  }
}

function loadReplayData() {
  if (!replayDataPath) return;
  try {
    const entries = JSON.parse(fs.readFileSync(replayDataPath, 'utf8'));
    for (const entry of entries) {
      const t = entry.type || '';
      if (!replayData[t]) replayData[t] = [];
      replayData[t].push(entry.attributes || {});
    }
  } catch (err) {
    process.stderr.write(`[aperio] Error loading replay data: ${err.message}\n`);
  }
}

// Save original fs methods before patching
fs.__aperio_origWriteFileSync = fs.writeFileSync;
fs.__aperio_origReadFileSync = fs.readFileSync;

// Load replay data
if (mode === 'replay') loadReplayData();

// Install all patches
patchChildProcess();
patchFilesystem();
patchNetwork();
patchDatabases();

// Flush on exit
process.on('exit', flushSpans);
process.on('SIGINT', () => { flushSpans(); process.exit(0); });
process.on('SIGTERM', () => { flushSpans(); process.exit(0); });
