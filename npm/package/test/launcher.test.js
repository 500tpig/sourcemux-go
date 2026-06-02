'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { EventEmitter } = require('node:events');
const test = require('node:test');
const assert = require('node:assert/strict');

const {
  MissingOptionalDependencyError,
  exitLikeChild,
  resolveBinary,
  runBinary
} = require('../lib/launcher');
const {
  SUPPORTED_PLATFORMS,
  UnsupportedPlatformError,
  getPlatformInfo
} = require('../lib/platform');
const { stagePlatformBinary } = require('../../scripts/stage-platform-binary');

test('maps Node platform and architecture to platform packages', () => {
  assert.deepEqual(getPlatformInfo('darwin', 'x64'), SUPPORTED_PLATFORMS['darwin-x64']);
  assert.deepEqual(getPlatformInfo('darwin', 'arm64'), SUPPORTED_PLATFORMS['darwin-arm64']);
  assert.deepEqual(getPlatformInfo('linux', 'x64'), SUPPORTED_PLATFORMS['linux-x64']);
  assert.deepEqual(getPlatformInfo('linux', 'arm64'), SUPPORTED_PLATFORMS['linux-arm64']);
  assert.equal(getPlatformInfo('win32', 'x64').binaryName, 'sourcemux.exe');

  assert.throws(
    () => getPlatformInfo('freebsd', 'x64'),
    (error) => {
      assert.ok(error instanceof UnsupportedPlatformError);
      assert.equal(error.code, 'SOURCEMUX_UNSUPPORTED_PLATFORM');
      assert.match(error.message, /darwin-arm64/);
      return true;
    }
  );
});

test('root package exposes sourcemux bin and all optional platform packages', () => {
  const manifest = JSON.parse(
    fs.readFileSync(path.resolve(__dirname, '..', 'package.json'), 'utf8')
  );
  const expectedOptionalPackages = Object.values(SUPPORTED_PLATFORMS)
    .map((info) => info.packageName)
    .sort();

  assert.equal(manifest.name, 'sourcemux');
  assert.deepEqual(manifest.bin, { sourcemux: './bin/sourcemux.js' });
  assert.deepEqual(Object.keys(manifest.optionalDependencies).sort(), expectedOptionalPackages);
});

test('platform package manifests match the launcher matrix', () => {
  const platformsDir = path.resolve(__dirname, '..', '..', 'platforms');

  for (const info of Object.values(SUPPORTED_PLATFORMS)) {
    const manifestPath = path.join(platformsDir, info.key, 'package.json');
    const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));

    assert.equal(manifest.name, info.packageName);
    assert.equal(manifest.version, '0.0.0-development');
    assert.deepEqual(manifest.os, [info.platform]);
    assert.deepEqual(manifest.cpu, [info.arch]);
    assert.equal(manifest.private, true);
  }
});

test('resolveBinary reports omitted or missing optional dependencies clearly', () => {
  const resolver = () => {
    const error = new Error('Cannot find module');
    error.code = 'MODULE_NOT_FOUND';
    throw error;
  };

  assert.throws(
    () => resolveBinary({ platform: 'linux', arch: 'x64', resolver }),
    (error) => {
      assert.ok(error instanceof MissingOptionalDependencyError);
      assert.equal(error.code, 'SOURCEMUX_MISSING_OPTIONAL_DEPENDENCY');
      assert.match(error.message, /@500tpig\/sourcemux-linux-x64/);
      assert.match(error.message, /--omit=optional/);
      assert.match(error.message, /copied from another platform/);
      return true;
    }
  );
});

test('resolveBinary returns the binary path for the selected platform package', () => {
  const expectedRequest = '@500tpig/sourcemux-win32-x64/bin/sourcemux.exe';
  const expectedPath = path.join('node_modules', '@500tpig', 'sourcemux-win32-x64', 'bin', 'sourcemux.exe');

  const result = resolveBinary({
    platform: 'win32',
    arch: 'x64',
    resolver(request) {
      assert.equal(request, expectedRequest);
      return expectedPath;
    }
  });

  assert.equal(result.binaryRequest, expectedRequest);
  assert.equal(result.binaryPath, expectedPath);
});

test('runBinary forwards command arguments and returns the child exit code', async () => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sourcemux-npm-test-'));
  const fixturePath = path.join(tempDir, 'mock-sourcemux.js');
  const outputPath = path.join(tempDir, 'args.json');

  fs.writeFileSync(
    fixturePath,
    [
      "'use strict';",
      "const fs = require('node:fs');",
      "fs.writeFileSync(process.env.SOURCEMUX_TEST_OUTPUT, JSON.stringify(process.argv.slice(2)));",
      "process.exit(Number(process.env.SOURCEMUX_TEST_EXIT || '0'));"
    ].join('\n')
  );

  const result = await runBinary(process.execPath, [fixturePath, '--version', '--json'], {
    env: {
      ...process.env,
      SOURCEMUX_TEST_OUTPUT: outputPath,
      SOURCEMUX_TEST_EXIT: '7'
    },
    stdio: 'ignore'
  });

  assert.equal(result.code, 7);
  assert.equal(result.signal, null);
  assert.deepEqual(JSON.parse(fs.readFileSync(outputPath, 'utf8')), ['--version', '--json']);
});

test('runBinary defaults to inherited stdio when spawning the native binary', async () => {
  const child = new EventEmitter();
  child.once = child.once.bind(child);
  let spawnOptions = null;

  const resultPromise = runBinary('/mock/sourcemux', ['version'], {
    spawn(_command, _args, options) {
      spawnOptions = options;
      process.nextTick(() => child.emit('exit', 0, null));
      return child;
    }
  });

  const result = await resultPromise;

  assert.equal(spawnOptions.stdio, 'inherit');
  assert.equal(result.code, 0);
});

test('exitLikeChild maps child signals to conventional fallback exit codes', () => {
  const calls = [];

  exitLikeChild(
    { code: null, signal: 'SIGTERM' },
    {
      pid: 123,
      kill(pid, signal) {
        calls.push(['kill', pid, signal]);
      },
      setTimeout(callback, delay) {
        calls.push(['setTimeout', delay]);
        callback();
        return { unref() {} };
      },
      exit(code) {
        calls.push(['exit', code]);
      }
    }
  );

  assert.deepEqual(calls, [
    ['kill', 123, 'SIGTERM'],
    ['setTimeout', 100],
    ['exit', 143]
  ]);
});

test('stagePlatformBinary writes only to the selected platform package bin path', (t) => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sourcemux-stage-test-'));
  t.after(() => fs.rmSync(tempDir, { recursive: true, force: true }));

  const sourcePath = path.join(tempDir, 'source-binary');
  const platformsRoot = path.join(tempDir, 'platforms');
  fs.writeFileSync(sourcePath, 'fake sourcemux binary');

  const destPath = stagePlatformBinary({
    target: 'win32-x64',
    binary: sourcePath,
    platformsRoot
  });

  assert.equal(
    destPath,
    path.join(platformsRoot, 'win32-x64', 'bin', 'sourcemux.exe')
  );
  assert.equal(fs.readFileSync(destPath, 'utf8'), 'fake sourcemux binary');
});

test('stagePlatformBinary rejects prototype or unsupported target names', () => {
  assert.throws(
    () => stagePlatformBinary({ target: 'toString', binary: __filename }),
    /Unsupported target/
  );
});
