#!/usr/bin/env node

import { existsSync, readdirSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import process from 'node:process';
import { spawnSync } from 'node:child_process';

const workspaceRoot = process.cwd();

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: workspaceRoot,
    env: {
      ...process.env,
      CI: process.env.CI ?? '1',
      NX_DAEMON: process.env.NX_DAEMON ?? 'false',
    },
    stdio: options.capture ? ['ignore', 'pipe', 'pipe'] : 'inherit',
    encoding: 'utf8',
  });

  if (options.capture) {
    if (result.status !== 0) {
      const stderr = result.stderr?.trim();
      throw new Error(stderr || `${command} ${args.join(' ')} failed`);
    }

    return result.stdout.trim();
  }

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

function resolveBaseRef() {
  if (process.env.CI_BASE_REF) {
    return process.env.CI_BASE_REF;
  }

  try {
    return run('git', ['rev-parse', '--abbrev-ref', '--symbolic-full-name', '@{upstream}'], { capture: true });
  } catch {
    return 'origin/main';
  }
}

function getMergeBase(baseRef) {
  return run('git', ['merge-base', 'HEAD', baseRef], { capture: true });
}

function getChangedFiles(mergeBase) {
  const output = run('git', ['diff', '--name-only', '--diff-filter=ACMR', `${mergeBase}...HEAD`], { capture: true });
  return output ? output.split('\n').map((file) => file.trim()).filter(Boolean) : [];
}

function hasExtension(filePath, extensions) {
  return extensions.some((extension) => filePath.endsWith(extension));
}

function isRootFrontendTestFile(filePath) {
  if (!hasExtension(filePath, ['.js', '.jsx', '.ts', '.tsx'])) {
    return false;
  }

  if (filePath.startsWith('packages/') || filePath.startsWith('public/app/plugins/') || filePath.startsWith('e2e-playwright/test-plugins/')) {
    return false;
  }

  return (
    filePath.startsWith('public/') ||
    filePath.startsWith('emails/') ||
    filePath.startsWith('scripts/') ||
    filePath === 'i18next.config.ts' ||
    filePath === 'playwright.config.ts' ||
    filePath === 'playwright.storybook.config.ts'
  );
}

function hasFrontendConfigChange(files) {
  return files.some((filePath) => {
    if (filePath === 'package.json' || filePath === 'yarn.lock' || filePath === 'nx.json' || filePath === 'tsconfig.json') {
      return true;
    }

    if (filePath === 'eslint.config.js' || filePath === 'knip.config.ts' || filePath === 'i18next.config.ts') {
      return true;
    }

    if (filePath === 'playwright.config.ts' || filePath === 'playwright.storybook.config.ts') {
      return true;
    }

    return filePath.startsWith('.yarn/');
  });
}

function hasFrontendPackageChange(files) {
  return files.some((filePath) => filePath.startsWith('packages/') || filePath.startsWith('public/app/plugins/') || filePath.startsWith('e2e-playwright/test-plugins/'));
}

function isGoGlobalChange(filePath) {
  if (filePath === 'go.mod' || filePath === 'go.sum' || filePath === 'go.work' || filePath === 'go.work.sum') {
    return true;
  }

  if (filePath === 'Makefile' || filePath === '.golangci.yml' || filePath === 'cue.mod' || filePath === 'embed.go') {
    return true;
  }

  if (filePath.startsWith('.citools/') || filePath.startsWith('scripts/go-workspace/') || filePath.startsWith('kinds/')) {
    return true;
  }

  return filePath.endsWith('.cue');
}

function mayAffectGo(filePath) {
  return (
    filePath.startsWith('pkg/') ||
    filePath.startsWith('apps/') ||
    filePath.endsWith('.go') ||
    filePath.endsWith('/go.mod') ||
    filePath.endsWith('/go.sum') ||
    isGoGlobalChange(filePath)
  );
}

function directoryHasGoFiles(directoryPath) {
  if (!existsSync(directoryPath)) {
    return false;
  }

  return readdirSync(directoryPath, { withFileTypes: true }).some((entry) => entry.isFile() && entry.name.endsWith('.go'));
}

function directoryHasGoMod(directoryPath) {
  return existsSync(join(directoryPath, 'go.mod'));
}

function normalizePath(directoryPath, recursive) {
  const relativeDir = relative(workspaceRoot, directoryPath).replace(/\\/g, '/');
  const base = relativeDir === '' ? '.' : `./${relativeDir}`;
  return recursive ? `${base}/...` : base;
}

function findNearestGoTarget(filePath) {
  let current = resolve(workspaceRoot, dirname(filePath));
  const root = resolve(workspaceRoot);

  while (current.startsWith(root)) {
    if (directoryHasGoFiles(current)) {
      return normalizePath(current, false);
    }

    if (directoryHasGoMod(current)) {
      return normalizePath(current, true);
    }

    if (current === root) {
      break;
    }

    current = dirname(current);
  }

  return './pkg/...';
}

function collectGoTargets(files) {
  const targets = new Set();

  for (const filePath of files) {
    if (!mayAffectGo(filePath) || isGoGlobalChange(filePath)) {
      continue;
    }

    targets.add(findNearestGoTarget(filePath));
  }

  return [...targets].sort();
}

function runGoTests(files) {
  const goFiles = files.filter(mayAffectGo);

  if (goFiles.length === 0) {
    return;
  }

  if (goFiles.some(isGoGlobalChange)) {
    run('yarn', ['nx', 'run', 'grafana:test-go-unit']);
    return;
  }

  const targets = collectGoTargets(goFiles);
  if (targets.length === 0) {
    run('yarn', ['nx', 'run', 'grafana:test-go-unit']);
    return;
  }

  run('go', ['test', '-short', '-timeout=30m', ...targets]);
}

function runFrontendTests(files, mergeBase) {
  const frontendRootFiles = files.filter(isRootFrontendTestFile);
  const hasPackageChange = hasFrontendPackageChange(files);
  const hasConfigChange = hasFrontendConfigChange(files);

  if (!frontendRootFiles.length && !hasPackageChange && !hasConfigChange) {
    return;
  }

  if (hasConfigChange) {
    run('yarn', ['nx', 'run', 'grafana:test:ci']);
  } else if (frontendRootFiles.length > 0) {
    run('yarn', ['run', 'test:ci', '--findRelatedTests', ...frontendRootFiles]);
  }

  if (hasPackageChange || hasConfigChange) {
    run('yarn', ['nx', 'affected', '-t', 'test:ci', `--base=${mergeBase}`, '--head=HEAD', '--exclude=grafana']);
  }
}

const baseRef = resolveBaseRef();
const mergeBase = getMergeBase(baseRef);
const changedFiles = getChangedFiles(mergeBase);

if (changedFiles.length === 0) {
  process.exit(0);
}

runGoTests(changedFiles);
runFrontendTests(changedFiles, mergeBase);
