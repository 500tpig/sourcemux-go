#!/usr/bin/env node
'use strict';

const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const { SUPPORTED_PLATFORMS } = require('../package/lib/platform');
const { stagePlatformBinary } = require('./stage-platform-binary');
const { normalizeVersion } = require('./set-package-version');

const RELEASE_TARGET_ORDER = Object.freeze([
  'darwin-arm64',
  'darwin-x64',
  'linux-arm64',
  'linux-x64',
  'win32-x64'
]);

const RELEASE_ASSETS_BY_TARGET = Object.freeze({
  'darwin-arm64': Object.freeze({ suffix: 'darwin_arm64', extension: 'tar.gz' }),
  'darwin-x64': Object.freeze({ suffix: 'darwin_amd64', extension: 'tar.gz' }),
  'linux-arm64': Object.freeze({ suffix: 'linux_arm64', extension: 'tar.gz' }),
  'linux-x64': Object.freeze({ suffix: 'linux_amd64', extension: 'tar.gz' }),
  'win32-x64': Object.freeze({ suffix: 'windows_amd64', extension: 'zip' })
});

function usage() {
  return [
    'Usage:',
    '  node npm/scripts/stage-release-binaries.js --version <version> --assets-dir <path>',
    '',
    'Extracts the versioned GitHub Release archives produced by GoReleaser and',
    'stages the native sourcemux binary into each npm platform package.',
    '',
    'Expected release assets:',
    '  sourcemux_<version>_darwin_arm64.tar.gz',
    '  sourcemux_<version>_darwin_amd64.tar.gz',
    '  sourcemux_<version>_linux_arm64.tar.gz',
    '  sourcemux_<version>_linux_amd64.tar.gz',
    '  sourcemux_<version>_windows_amd64.zip'
  ].join('\n');
}

function parseArgs(argv) {
  const parsed = {};
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === '--help' || arg === '-h') {
      parsed.help = true;
      continue;
    }
    if (arg === '--version' || arg === '--assets-dir' || arg === '--platforms-root') {
      const value = argv[index + 1];
      if (!value) {
        throw new Error(`Missing value for ${arg}`);
      }
      parsed[arg.slice(2).replace(/-([a-z])/g, (_, char) => char.toUpperCase())] = value;
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }
  return parsed;
}

function assetNameForTarget(target, versionInput) {
  const info = SUPPORTED_PLATFORMS[target];
  const asset = RELEASE_ASSETS_BY_TARGET[target];
  if (!info || !asset) {
    throw new Error(`Unsupported release target: ${target}`);
  }

  const version = normalizeVersion(versionInput);
  return `sourcemux_${version}_${asset.suffix}.${asset.extension}`;
}

function runCommand(command, args) {
  const result = spawnSync(command, args, {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe']
  });

  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(
      `${command} ${args.join(' ')} failed\n${result.stderr || result.stdout}`
    );
  }
}

function extractArchive(archivePath, destination) {
  if (archivePath.endsWith('.zip')) {
    runCommand('unzip', ['-q', archivePath, '-d', destination]);
    return;
  }
  if (archivePath.endsWith('.tar.gz')) {
    runCommand('tar', ['-xzf', archivePath, '-C', destination]);
    return;
  }
  throw new Error(`Unsupported release archive extension: ${archivePath}`);
}

function walkFiles(root, files = []) {
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const absolutePath = path.join(root, entry.name);
    if (entry.isDirectory()) {
      walkFiles(absolutePath, files);
    } else if (entry.isFile()) {
      files.push(absolutePath);
    }
  }
  return files;
}

function findExtractedBinary(root, binaryName) {
  const matches = walkFiles(root).filter((filePath) => path.basename(filePath) === binaryName);
  if (matches.length === 0) {
    throw new Error(`Release archive did not contain expected binary: ${binaryName}`);
  }
  if (matches.length > 1) {
    throw new Error(`Release archive contained multiple ${binaryName} files: ${matches.join(', ')}`);
  }
  return matches[0];
}

function stageReleaseBinaries(options) {
  if (!options || typeof options !== 'object') {
    throw new Error('stageReleaseBinaries options are required');
  }
  const version = normalizeVersion(options.version);
  if (!options.assetsDir) {
    throw new Error('--assets-dir is required');
  }
  const assetsDir = path.resolve(options.assetsDir);
  if (!fs.statSync(assetsDir).isDirectory()) {
    throw new Error(`Release assets path is not a directory: ${assetsDir}`);
  }
  const platformsRoot = options.platformsRoot ? path.resolve(options.platformsRoot) : undefined;
  const staged = [];

  for (const target of RELEASE_TARGET_ORDER) {
    const info = SUPPORTED_PLATFORMS[target];
    const assetName = assetNameForTarget(target, version);
    const assetPath = path.join(assetsDir, assetName);
    if (!fs.existsSync(assetPath)) {
      throw new Error(`Missing release asset for ${target}: ${assetPath}`);
    }

    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `sourcemux-${target}-`));
    try {
      extractArchive(assetPath, tempDir);
      const binaryPath = findExtractedBinary(tempDir, info.binaryName);
      const stagedPath = stagePlatformBinary({
        target,
        binary: binaryPath,
        platformsRoot
      });
      staged.push({ target, assetName, stagedPath });
    } finally {
      fs.rmSync(tempDir, { recursive: true, force: true });
    }
  }

  return staged;
}

function main(argv = process.argv.slice(2)) {
  let parsed;
  try {
    parsed = parseArgs(argv);
    if (parsed.help) {
      process.stdout.write(`${usage()}\n`);
      return 0;
    }
    if (!parsed.version || !parsed.assetsDir) {
      throw new Error('--version and --assets-dir are required');
    }

    const staged = stageReleaseBinaries(parsed);
    for (const item of staged) {
      process.stdout.write(`${item.target}: ${item.assetName} -> ${item.stagedPath}\n`);
    }
    return 0;
  } catch (error) {
    process.stderr.write(`${error.message}\n\n${usage()}\n`);
    return 1;
  }
}

if (require.main === module) {
  process.exit(main());
}

module.exports = {
  RELEASE_ASSETS_BY_TARGET,
  RELEASE_TARGET_ORDER,
  assetNameForTarget,
  extractArchive,
  findExtractedBinary,
  parseArgs,
  stageReleaseBinaries
};
