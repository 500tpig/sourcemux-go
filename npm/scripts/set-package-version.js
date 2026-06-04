#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const { SUPPORTED_PLATFORMS } = require('../package/lib/platform');

function usage() {
  return [
    'Usage:',
    '  node npm/scripts/set-package-version.js --version <version>',
    '',
    'Sets npm/package/package.json, every npm/platforms/*/package.json, and',
    'root optionalDependencies to the release version. Pass a tag with or',
    'without the leading "v".'
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
    if (arg === '--version') {
      const value = argv[index + 1];
      if (!value) {
        throw new Error(`Missing value for ${arg}`);
      }
      parsed.version = value;
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }
  return parsed;
}

function normalizeVersion(input) {
  if (!input || typeof input !== 'string') {
    throw new Error('Version is required');
  }

  const version = input.startsWith('v') ? input.slice(1) : input;
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`Invalid npm release version: ${input}`);
  }
  return version;
}

function readJSON(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function writeJSON(filePath, value) {
  fs.writeFileSync(filePath, `${JSON.stringify(value, null, 2)}\n`);
}

function setPackageVersion(options) {
  const version = normalizeVersion(options.version);
  const repoRoot = path.resolve(options.repoRoot || path.resolve(__dirname, '..', '..'));
  const rootManifestPath = path.join(repoRoot, 'npm', 'package', 'package.json');
  const rootManifest = readJSON(rootManifestPath);

  rootManifest.version = version;
  if (!rootManifest.optionalDependencies || typeof rootManifest.optionalDependencies !== 'object') {
    throw new Error('Root npm package is missing optionalDependencies');
  }

  const updated = [rootManifestPath];
  for (const info of Object.values(SUPPORTED_PLATFORMS)) {
    rootManifest.optionalDependencies[info.packageName] = version;
  }
  writeJSON(rootManifestPath, rootManifest);

  for (const info of Object.values(SUPPORTED_PLATFORMS)) {
    const manifestPath = path.join(repoRoot, 'npm', 'platforms', info.key, 'package.json');
    const manifest = readJSON(manifestPath);
    manifest.version = version;
    writeJSON(manifestPath, manifest);
    updated.push(manifestPath);
  }

  return updated;
}

function main(argv = process.argv.slice(2)) {
  let parsed;
  try {
    parsed = parseArgs(argv);
    if (parsed.help) {
      process.stdout.write(`${usage()}\n`);
      return 0;
    }
    if (!parsed.version) {
      throw new Error('--version is required');
    }

    const updated = setPackageVersion({ version: parsed.version });
    for (const filePath of updated) {
      process.stdout.write(`Updated ${filePath}\n`);
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
  normalizeVersion,
  parseArgs,
  setPackageVersion
};
