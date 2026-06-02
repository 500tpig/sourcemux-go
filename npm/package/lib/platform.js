'use strict';

const SUPPORTED_PLATFORMS = Object.freeze({
  'darwin-x64': Object.freeze({
    key: 'darwin-x64',
    platform: 'darwin',
    arch: 'x64',
    goos: 'darwin',
    goarch: 'amd64',
    packageName: '@500tpig/sourcemux-darwin-x64',
    binaryName: 'sourcemux'
  }),
  'darwin-arm64': Object.freeze({
    key: 'darwin-arm64',
    platform: 'darwin',
    arch: 'arm64',
    goos: 'darwin',
    goarch: 'arm64',
    packageName: '@500tpig/sourcemux-darwin-arm64',
    binaryName: 'sourcemux'
  }),
  'linux-x64': Object.freeze({
    key: 'linux-x64',
    platform: 'linux',
    arch: 'x64',
    goos: 'linux',
    goarch: 'amd64',
    packageName: '@500tpig/sourcemux-linux-x64',
    binaryName: 'sourcemux'
  }),
  'linux-arm64': Object.freeze({
    key: 'linux-arm64',
    platform: 'linux',
    arch: 'arm64',
    goos: 'linux',
    goarch: 'arm64',
    packageName: '@500tpig/sourcemux-linux-arm64',
    binaryName: 'sourcemux'
  }),
  'win32-x64': Object.freeze({
    key: 'win32-x64',
    platform: 'win32',
    arch: 'x64',
    goos: 'windows',
    goarch: 'amd64',
    packageName: '@500tpig/sourcemux-win32-x64',
    binaryName: 'sourcemux.exe'
  })
});

class UnsupportedPlatformError extends Error {
  constructor(platform, arch) {
    super(
      `Unsupported platform "${platform}" and architecture "${arch}". ` +
        `Supported npm wrapper targets: ${supportedPlatformSummary()}. ` +
        'Build SourceMux from source or install from GitHub Releases for this platform.'
    );
    this.name = 'UnsupportedPlatformError';
    this.code = 'SOURCEMUX_UNSUPPORTED_PLATFORM';
    this.platform = platform;
    this.arch = arch;
  }
}

function keyForPlatform(platform, arch) {
  return `${platform}-${arch}`;
}

function supportedPlatformSummary() {
  return Object.keys(SUPPORTED_PLATFORMS).sort().join(', ');
}

function getPlatformInfo(platform = process.platform, arch = process.arch) {
  const key = keyForPlatform(platform, arch);
  const info = SUPPORTED_PLATFORMS[key];
  if (!info) {
    throw new UnsupportedPlatformError(platform, arch);
  }
  return info;
}

module.exports = {
  SUPPORTED_PLATFORMS,
  UnsupportedPlatformError,
  getPlatformInfo,
  keyForPlatform,
  supportedPlatformSummary
};
