'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { EventEmitter } = require('node:events');
const { spawnSync } = require('node:child_process');
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
const {
  RELEASE_ASSETS_BY_TARGET,
  RELEASE_TARGET_ORDER,
  assetNameForTarget,
  stageReleaseBinaries
} = require('../../scripts/stage-release-binaries');
const {
  normalizeVersion,
  setPackageVersion
} = require('../../scripts/set-package-version');
const {
  verifyPlatformPackage,
  verifyRootPackage
} = require('../../scripts/verify-pack-dry-run');

const expectedRepositoryURL = 'https://github.com/500tpig/sourcemux-go';

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
  assert.equal(manifest.private, undefined);
  assert.deepEqual(manifest.bin, { sourcemux: 'bin/sourcemux.js' });
  assert.equal(manifest.repository.url, expectedRepositoryURL);
  assert.deepEqual(Object.keys(manifest.optionalDependencies).sort(), expectedOptionalPackages);

  for (const packageVersion of Object.values(manifest.optionalDependencies)) {
    assert.equal(packageVersion, manifest.version);
  }
});

test('platform package manifests match the launcher matrix', () => {
  const platformsDir = path.resolve(__dirname, '..', '..', 'platforms');
  const rootManifest = JSON.parse(
    fs.readFileSync(path.resolve(__dirname, '..', 'package.json'), 'utf8')
  );

  for (const info of Object.values(SUPPORTED_PLATFORMS)) {
    const manifestPath = path.join(platformsDir, info.key, 'package.json');
    const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));

    assert.equal(manifest.name, info.packageName);
    assert.equal(manifest.version, rootManifest.version);
    assert.deepEqual(manifest.os, [info.platform]);
    assert.deepEqual(manifest.cpu, [info.arch]);
    assert.equal(manifest.private, undefined);
    assert.equal(manifest.repository.url, expectedRepositoryURL);
    assert.equal(manifest.repository.directory, `npm/platforms/${info.key}`);
    assert.deepEqual(manifest.publishConfig, { access: 'public' });
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

test('setPackageVersion aligns root and platform package versions', (t) => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sourcemux-version-test-'));
  t.after(() => fs.rmSync(tempDir, { recursive: true, force: true }));

  const rootPackageDir = path.join(tempDir, 'npm', 'package');
  const platformsDir = path.join(tempDir, 'npm', 'platforms');
  fs.mkdirSync(rootPackageDir, { recursive: true });
  fs.writeFileSync(path.join(rootPackageDir, 'package.json'), JSON.stringify({
    name: 'sourcemux',
    version: '0.0.0',
    optionalDependencies: Object.fromEntries(
      Object.values(SUPPORTED_PLATFORMS).map((info) => [info.packageName, '0.0.0'])
    )
  }));

  for (const info of Object.values(SUPPORTED_PLATFORMS)) {
    const packageDir = path.join(platformsDir, info.key);
    fs.mkdirSync(packageDir, { recursive: true });
    fs.writeFileSync(path.join(packageDir, 'package.json'), JSON.stringify({
      name: info.packageName,
      version: '0.0.0'
    }));
  }

  const updated = setPackageVersion({ version: 'v1.2.3', repoRoot: tempDir });
  assert.equal(updated.length, 1 + Object.keys(SUPPORTED_PLATFORMS).length);

  const rootManifest = JSON.parse(
    fs.readFileSync(path.join(rootPackageDir, 'package.json'), 'utf8')
  );
  assert.equal(rootManifest.version, '1.2.3');
  for (const info of Object.values(SUPPORTED_PLATFORMS)) {
    assert.equal(rootManifest.optionalDependencies[info.packageName], '1.2.3');
    const manifest = JSON.parse(
      fs.readFileSync(path.join(platformsDir, info.key, 'package.json'), 'utf8')
    );
    assert.equal(manifest.version, '1.2.3');
  }
});

test('normalizeVersion accepts release tags and rejects invalid versions', () => {
  assert.equal(normalizeVersion('v1.2.3'), '1.2.3');
  assert.equal(normalizeVersion('1.2.3-beta.1'), '1.2.3-beta.1');
  assert.throws(() => normalizeVersion('latest'), /Invalid npm release version/);
});

test('release asset mapping matches the GoReleaser archive names', () => {
  assert.deepEqual(RELEASE_TARGET_ORDER, [
    'darwin-arm64',
    'darwin-x64',
    'linux-arm64',
    'linux-x64',
    'win32-x64'
  ]);
  assert.deepEqual(Object.keys(RELEASE_ASSETS_BY_TARGET).sort(), RELEASE_TARGET_ORDER.slice().sort());
  assert.equal(assetNameForTarget('darwin-arm64', 'v1.2.3'), 'sourcemux_1.2.3_darwin_arm64.tar.gz');
  assert.equal(assetNameForTarget('darwin-x64', 'v1.2.3'), 'sourcemux_1.2.3_darwin_amd64.tar.gz');
  assert.equal(assetNameForTarget('linux-arm64', 'v1.2.3'), 'sourcemux_1.2.3_linux_arm64.tar.gz');
  assert.equal(assetNameForTarget('linux-x64', 'v1.2.3'), 'sourcemux_1.2.3_linux_amd64.tar.gz');
  assert.equal(assetNameForTarget('win32-x64', 'v1.2.3'), 'sourcemux_1.2.3_windows_amd64.zip');
});

test('stageReleaseBinaries maps GoReleaser assets to npm platform packages', (t) => {
  if (!commandAvailable('tar', ['--version']) ||
      !commandAvailable('zip', ['-v']) ||
      !commandAvailable('unzip', ['-v'])) {
    t.skip('tar, zip, or unzip is not available');
    return;
  }

  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sourcemux-release-test-'));
  t.after(() => fs.rmSync(tempDir, { recursive: true, force: true }));

  const assetsDir = path.join(tempDir, 'assets');
  const platformsRoot = path.join(tempDir, 'platforms');
  fs.mkdirSync(assetsDir, { recursive: true });

  for (const target of RELEASE_TARGET_ORDER) {
    const info = SUPPORTED_PLATFORMS[target];
    const sourceDir = path.join(tempDir, 'sources', target);
    fs.mkdirSync(sourceDir, { recursive: true });
    fs.writeFileSync(path.join(sourceDir, info.binaryName), `binary for ${target}`);

    const archivePath = path.join(assetsDir, assetNameForTarget(target, 'v1.2.3'));
    if (archivePath.endsWith('.zip')) {
      const result = spawnSync('zip', ['-q', archivePath, info.binaryName], {
        cwd: sourceDir
      });
      assert.equal(result.status, 0, result.stderr && result.stderr.toString());
    } else {
      const result = spawnSync('tar', ['-czf', archivePath, '-C', sourceDir, info.binaryName]);
      assert.equal(result.status, 0, result.stderr && result.stderr.toString());
    }
  }

  const staged = stageReleaseBinaries({
    version: 'v1.2.3',
    assetsDir,
    platformsRoot
  });

  assert.deepEqual(staged.map((item) => item.target), RELEASE_TARGET_ORDER);
  for (const target of RELEASE_TARGET_ORDER) {
    const info = SUPPORTED_PLATFORMS[target];
    const stagedPath = path.join(platformsRoot, target, 'bin', info.binaryName);
    assert.equal(fs.readFileSync(stagedPath, 'utf8'), `binary for ${target}`);
  }
});

test('verifyRootPackage accepts only the expected wrapper files', () => {
  assert.doesNotThrow(() => verifyRootPackage({
    files: [
      { path: 'README.md', mode: 0o644 },
      { path: 'bin/sourcemux.js', mode: 0o755 },
      { path: 'lib/launcher.js', mode: 0o644 },
      { path: 'lib/platform.js', mode: 0o644 },
      { path: 'package.json', mode: 0o644 }
    ]
  }));

  assert.throws(
    () => verifyRootPackage({
      files: [
        { path: 'README.md', mode: 0o644 },
        { path: 'bin/sourcemux.js', mode: 0o755 },
        { path: 'lib/launcher.js', mode: 0o644 },
        { path: 'lib/platform.js', mode: 0o644 },
        { path: 'package.json', mode: 0o644 },
        { path: 'sourcemux.json', mode: 0o600 }
      ]
    }),
    /Forbidden npm pack entry/
  );
});

function commandAvailable(command, args) {
  const result = spawnSync(command, args, { stdio: 'ignore' });
  return !result.error && result.status === 0;
}

test('verifyPlatformPackage allows staged binary paths and can require them', () => {
  const info = SUPPORTED_PLATFORMS['darwin-arm64'];
  const result = {
    files: [
      { path: 'package.json', mode: 0o644 },
      { path: 'bin/sourcemux', mode: 0o755 }
    ]
  };

  assert.doesNotThrow(() => verifyPlatformPackage(info, result, {
    requireStagedBinaries: true
  }));

  assert.throws(
    () => verifyPlatformPackage(info, {
      files: [
        { path: 'package.json', mode: 0o644 },
        { path: 'bin/grok-search', mode: 0o755 }
      ]
    }, {
      requireStagedBinaries: false
    }),
    /Unexpected npm pack entry/
  );

  assert.throws(
    () => verifyPlatformPackage(info, {
      files: [
        { path: 'package.json', mode: 0o644 }
      ]
    }, {
      requireStagedBinaries: true
    }),
    /missing staged binary/
  );
});
