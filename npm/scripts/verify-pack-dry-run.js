#!/usr/bin/env node
'use strict';

const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const { SUPPORTED_PLATFORMS } = require('../package/lib/platform');

const ROOT_ALLOWED_FILES = new Set([
  'README.md',
  'bin/sourcemux.js',
  'lib/launcher.js',
  'lib/platform.js',
  'package.json'
]);

const FORBIDDEN_PATTERNS = [
  /\.env(?:\.|$)/i,
  /(?:^|\/)(?:sourcemux|grok-search)\.json$/i,
  /(?:^|\/)config\.local\.json$/i,
  /(?:^|\/)npmrc$/i,
  /(?:^|\/)\.npmrc$/i,
  /NPM_TOKEN/i,
  /api[-_]?key/i,
  /dashboard/i,
  /private[-_]?endpoint/i,
  /\.tgz$/i
];

const FORBIDDEN_CONTENT_PATTERNS = [
  /NPM_TOKEN/i,
  /npm_[A-Za-z0-9]{20,}/,
  /sk-[A-Za-z0-9_-]{20,}/,
  /provider dashboard export/i,
  /private endpoint/i
];

function usage() {
  return [
    'Usage:',
    '  node npm/scripts/verify-pack-dry-run.js [--require-staged-binaries]',
    '',
    'Runs npm pack --dry-run --json for npm/package and every npm/platforms/*',
    'package, then rejects unexpected files, local configs, tokens, package',
    'artifacts, and other publish-risk paths.',
    '',
    'Use --require-staged-binaries during release packaging after staging each',
    'native sourcemux binary into npm/platforms/<target>/bin/.'
  ].join('\n');
}

function parseArgs(argv) {
  const parsed = {
    requireStagedBinaries: false
  };

  for (const arg of argv) {
    if (arg === '--help' || arg === '-h') {
      parsed.help = true;
    } else if (arg === '--require-staged-binaries') {
      parsed.requireStagedBinaries = true;
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }

  return parsed;
}

function runPackDryRun(packageDir, cwd) {
  const result = spawnSync(
    'npm',
    ['pack', packageDir, '--dry-run', '--json'],
    {
      cwd,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe']
    }
  );

  if (result.status !== 0) {
    throw new Error(
      `npm pack dry-run failed for ${packageDir}\n${result.stderr || result.stdout}`
    );
  }

  let parsed;
  try {
    parsed = JSON.parse(result.stdout);
  } catch (error) {
    throw new Error(`Could not parse npm pack JSON for ${packageDir}: ${error.message}`);
  }

  if (!Array.isArray(parsed) || parsed.length !== 1) {
    throw new Error(`Expected one npm pack result for ${packageDir}`);
  }

  return parsed[0];
}

function assertNoForbiddenPath(packageDir, filePath) {
  for (const pattern of FORBIDDEN_PATTERNS) {
    if (pattern.test(filePath)) {
      throw new Error(`Forbidden npm pack entry in ${packageDir}: ${filePath}`);
    }
  }
}

function assertAllowedFiles(packageDir, files, allowedFiles) {
  for (const file of files) {
    assertNoForbiddenPath(packageDir, file.path);
    if (!allowedFiles.has(file.path)) {
      throw new Error(`Unexpected npm pack entry in ${packageDir}: ${file.path}`);
    }
  }
}

function shouldScanContent(filePath) {
  return !filePath.startsWith('bin/');
}

function assertNoForbiddenContent(packageDir, packageRoot, files) {
  if (!packageRoot) {
    return;
  }

  for (const file of files) {
    if (!shouldScanContent(file.path)) {
      continue;
    }

    const absolutePath = path.join(packageRoot, file.path);
    const content = fs.readFileSync(absolutePath, 'utf8');
    for (const pattern of FORBIDDEN_CONTENT_PATTERNS) {
      if (pattern.test(content)) {
        throw new Error(`Forbidden npm pack content in ${packageDir}: ${file.path}`);
      }
    }
  }
}

function binaryIsExecutable(file) {
  return (file.mode & 0o111) !== 0;
}

function verifyRootPackage(result, packageRoot) {
  assertAllowedFiles('npm/package', result.files, ROOT_ALLOWED_FILES);
  assertNoForbiddenContent('npm/package', packageRoot, result.files);

  const found = new Set(result.files.map((file) => file.path));
  for (const requiredFile of ROOT_ALLOWED_FILES) {
    if (!found.has(requiredFile)) {
      throw new Error(`Root npm package is missing expected file: ${requiredFile}`);
    }
  }
}

function verifyPlatformPackage(info, result, options, packageRoot) {
  const binaryPath = `bin/${info.binaryName}`;
  const allowedFiles = new Set(['package.json', binaryPath]);
  assertAllowedFiles(`npm/platforms/${info.key}`, result.files, allowedFiles);
  assertNoForbiddenContent(`npm/platforms/${info.key}`, packageRoot, result.files);

  const found = new Map(result.files.map((file) => [file.path, file]));
  if (!found.has('package.json')) {
    throw new Error(`Platform package ${info.key} is missing package.json`);
  }

  const binaryFile = found.get(binaryPath);
  if (options.requireStagedBinaries && !binaryFile) {
    throw new Error(`Platform package ${info.key} is missing staged binary ${binaryPath}`);
  }
  if (binaryFile && info.platform !== 'win32' && !binaryIsExecutable(binaryFile)) {
    throw new Error(`Platform package ${info.key} binary is not executable: ${binaryPath}`);
  }
}

function printSummary(packages) {
  for (const pkg of packages) {
    const files = pkg.result.files.map((file) => file.path).join(', ');
    process.stdout.write(`${pkg.label}: ${files}\n`);
  }
}

function main(argv = process.argv.slice(2), cwd = path.resolve(__dirname, '..', '..')) {
  const options = parseArgs(argv);
  if (options.help) {
    process.stdout.write(`${usage()}\n`);
    return 0;
  }

  const rootResult = runPackDryRun('./npm/package', cwd);
  verifyRootPackage(rootResult, path.join(cwd, 'npm', 'package'));

  const packages = [{ label: rootResult.id, result: rootResult }];

  for (const info of Object.values(SUPPORTED_PLATFORMS)) {
    const packageDir = `./npm/platforms/${info.key}`;
    if (!fs.existsSync(path.join(cwd, packageDir, 'package.json'))) {
      throw new Error(`Missing platform package manifest: ${packageDir}/package.json`);
    }

    const result = runPackDryRun(packageDir, cwd);
    verifyPlatformPackage(info, result, options, path.join(cwd, 'npm', 'platforms', info.key));
    packages.push({ label: result.id, result });
  }

  printSummary(packages);
  return 0;
}

if (require.main === module) {
  try {
    process.exit(main());
  } catch (error) {
    process.stderr.write(`${error.message}\n\n${usage()}\n`);
    process.exit(1);
  }
}

module.exports = {
  FORBIDDEN_PATTERNS,
  FORBIDDEN_CONTENT_PATTERNS,
  ROOT_ALLOWED_FILES,
  main,
  parseArgs,
  runPackDryRun,
  verifyPlatformPackage,
  verifyRootPackage
};
