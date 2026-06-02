#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const { SUPPORTED_PLATFORMS } = require('../package/lib/platform');

function hasSupportedTarget(target) {
  return Object.prototype.hasOwnProperty.call(SUPPORTED_PLATFORMS, target);
}

function usage() {
  const targets = Object.keys(SUPPORTED_PLATFORMS).sort().join(', ');
  return [
    'Usage:',
    '  node npm/scripts/stage-platform-binary.js --target <target> --binary <path>',
    '',
    `Targets: ${targets}`,
    '',
    'The script copies a locally built or extracted SourceMux binary into the',
    'matching npm platform package bin/ directory. Staged binaries are ignored',
    'by git and must not be committed.'
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
    if (arg === '--target' || arg === '--binary') {
      const value = argv[index + 1];
      if (!value) {
        throw new Error(`Missing value for ${arg}`);
      }
      parsed[arg.slice(2)] = value;
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }
  return parsed;
}

function assertInsideDirectory(directory, candidatePath) {
  const relative = path.relative(directory, candidatePath);
  if (!relative || relative.startsWith('..') || path.isAbsolute(relative)) {
    throw new Error(`Refusing to stage binary outside ${directory}: ${candidatePath}`);
  }
}

function stagePlatformBinary(options) {
  if (!hasSupportedTarget(options.target)) {
    throw new Error(`Unsupported target "${options.target}". Expected one of: ${Object.keys(SUPPORTED_PLATFORMS).sort().join(', ')}`);
  }

  const info = SUPPORTED_PLATFORMS[options.target];
  const sourcePath = path.resolve(options.binary);
  const sourceStat = fs.statSync(sourcePath);
  if (!sourceStat.isFile()) {
    throw new Error(`Binary path is not a file: ${sourcePath}`);
  }

  const platformsRoot = options.platformsRoot
    ? path.resolve(options.platformsRoot)
    : path.resolve(__dirname, '..', 'platforms');
  const packageDir = path.join(platformsRoot, info.key);
  const destDir = path.join(packageDir, 'bin');
  const destPath = path.join(destDir, info.binaryName);
  assertInsideDirectory(destDir, destPath);

  fs.mkdirSync(destDir, { recursive: true });
  fs.copyFileSync(sourcePath, destPath);

  if (info.platform !== 'win32') {
    fs.chmodSync(destPath, 0o755);
  }

  return destPath;
}

function main(argv = process.argv.slice(2)) {
  let parsed;
  try {
    parsed = parseArgs(argv);
    if (parsed.help) {
      process.stdout.write(`${usage()}\n`);
      return 0;
    }
    if (!parsed.target || !parsed.binary) {
      throw new Error('Both --target and --binary are required');
    }

    const destPath = stagePlatformBinary(parsed);
    process.stdout.write(`Staged SourceMux binary at ${destPath}\n`);
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
  parseArgs,
  stagePlatformBinary
};
