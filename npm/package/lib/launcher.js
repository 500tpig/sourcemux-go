'use strict';

const { spawn: defaultSpawn } = require('node:child_process');

const { getPlatformInfo } = require('./platform');

const DEFAULT_FORWARD_SIGNALS = Object.freeze(['SIGINT', 'SIGTERM', 'SIGHUP']);
const SIGNAL_EXIT_CODES = Object.freeze({
  SIGHUP: 129,
  SIGINT: 130,
  SIGTERM: 143
});

class MissingOptionalDependencyError extends Error {
  constructor(platformInfo, binaryRequest, cause) {
    super(missingOptionalDependencyMessage(platformInfo, binaryRequest));
    this.name = 'MissingOptionalDependencyError';
    this.code = 'SOURCEMUX_MISSING_OPTIONAL_DEPENDENCY';
    this.platformInfo = platformInfo;
    this.binaryRequest = binaryRequest;
    this.cause = cause;
  }
}

function missingOptionalDependencyMessage(platformInfo, binaryRequest) {
  return [
    `The native SourceMux package "${platformInfo.packageName}" is not installed for ` +
      `${platformInfo.platform}/${platformInfo.arch}.`,
    `The wrapper tried to resolve "${binaryRequest}".`,
    'This usually means optional dependencies were omitted, for example with ' +
      '`npm install --omit=optional`, `npm install --no-optional`, or a package-manager setting.',
    'It can also happen when node_modules was copied from another platform or architecture.',
    'Reinstall the root `sourcemux` package on this machine with optional dependencies enabled.'
  ].join('\n');
}

function resolveBinary(options = {}) {
  const platformInfo = getPlatformInfo(options.platform, options.arch);
  const binaryRequest = `${platformInfo.packageName}/bin/${platformInfo.binaryName}`;
  const resolver = options.resolver || require.resolve;

  try {
    return {
      binaryPath: resolver(binaryRequest),
      binaryRequest,
      platformInfo
    };
  } catch (error) {
    if (error && error.code === 'MODULE_NOT_FOUND') {
      throw new MissingOptionalDependencyError(platformInfo, binaryRequest, error);
    }
    throw error;
  }
}

function spawnBinary(binaryPath, args, options = {}) {
  const spawn = options.spawn || defaultSpawn;
  return spawn(binaryPath, args, {
    cwd: options.cwd || process.cwd(),
    env: options.env || process.env,
    stdio: options.stdio || 'inherit',
    windowsHide: false
  });
}

function runBinary(binaryPath, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawnBinary(binaryPath, args, options);
    let settled = false;

    if (typeof options.onChild === 'function') {
      options.onChild(child);
    }

    child.once('error', (error) => {
      if (!settled) {
        settled = true;
        reject(error);
      }
    });

    child.once('exit', (code, signal) => {
      if (!settled) {
        settled = true;
        resolve({ code, signal });
      }
    });
  });
}

function run(args, options = {}) {
  const resolved = options.binaryPath
    ? { binaryPath: options.binaryPath }
    : resolveBinary(options);
  return runBinary(resolved.binaryPath, args, options);
}

function installSignalForwarding(child, signals = DEFAULT_FORWARD_SIGNALS, proc = process) {
  const installed = [];

  for (const signal of signals) {
    const handler = () => {
      if (!child.killed) {
        try {
          child.kill(signal);
        } catch (_) {
          // The child may have exited between the signal and forwarding.
        }
      }
    };

    try {
      proc.once(signal, handler);
      installed.push([signal, handler]);
    } catch (_) {
      // Some signals are not supported on every OS/runtime combination.
    }
  }

  const cleanup = () => {
    for (const [signal, handler] of installed) {
      proc.removeListener(signal, handler);
    }
  };

  child.once('exit', cleanup);
  return cleanup;
}

function signalExitCode(signal) {
  return SIGNAL_EXIT_CODES[signal] || 1;
}

function exitLikeChild(result, controls = {}) {
  const exit = controls.exit || process.exit;

  if (result.signal) {
    const kill = controls.kill || process.kill;
    const pid = controls.pid || process.pid;
    try {
      kill(pid, result.signal);
    } catch (_) {
      exit(signalExitCode(result.signal));
      return;
    }

    const setTimeoutFn = controls.setTimeout || setTimeout;
    const timer = setTimeoutFn(() => exit(signalExitCode(result.signal)), 100);
    if (timer && typeof timer.unref === 'function') {
      timer.unref();
    }
    return;
  }

  exit(result.code === null || result.code === undefined ? 1 : result.code);
}

function formatLauncherError(error) {
  if (
    error &&
    (error.code === 'SOURCEMUX_MISSING_OPTIONAL_DEPENDENCY' ||
      error.code === 'SOURCEMUX_UNSUPPORTED_PLATFORM')
  ) {
    return `sourcemux npm wrapper error:\n${error.message}\n`;
  }

  const message = error && error.message ? error.message : String(error);
  return `sourcemux npm wrapper failed to start: ${message}\n`;
}

async function main(argv = process.argv.slice(2), options = {}) {
  const stderr = options.stderr || process.stderr;
  const processControls = options.processControls || {};

  try {
    let cleanupSignals = null;
    const result = await run(argv, {
      ...options,
      onChild(child) {
        cleanupSignals = installSignalForwarding(
          child,
          options.signals || DEFAULT_FORWARD_SIGNALS,
          options.processObject || process
        );
        if (typeof options.onChild === 'function') {
          options.onChild(child);
        }
      }
    });

    if (cleanupSignals) {
      cleanupSignals();
    }
    exitLikeChild(result, processControls);
  } catch (error) {
    stderr.write(formatLauncherError(error));
    const exit = processControls.exit || process.exit;
    exit(1);
  }
}

module.exports = {
  MissingOptionalDependencyError,
  DEFAULT_FORWARD_SIGNALS,
  exitLikeChild,
  formatLauncherError,
  installSignalForwarding,
  missingOptionalDependencyMessage,
  resolveBinary,
  run,
  runBinary,
  signalExitCode,
  spawnBinary,
  main
};
