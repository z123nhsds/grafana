#!/usr/bin/env node

import { spawn } from 'node:child_process';
import { createWriteStream, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import process from 'node:process';
import readline from 'node:readline';

const workspaceRoot = process.cwd();
const reportDir = join(workspaceRoot, 'reports', 'ci-fast');

mkdirSync(reportDir, { recursive: true });

const sharedEnv = {
  ...process.env,
  CI: process.env.CI ?? '1',
  NX_DAEMON: process.env.NX_DAEMON ?? 'false',
  TEST_MAX_WORKERS: process.env.TEST_MAX_WORKERS ?? '50%',
};

const tasks = [
  { name: 'lint-go', command: 'yarn nx run grafana:lint-go' },
  { name: 'lint-ts', command: 'yarn nx run grafana:lint' },
  { name: 'typecheck', command: 'yarn nx run grafana:typecheck' },
  { name: 'test-go-unit', command: 'yarn nx run grafana:test-go-unit' },
  { name: 'test-frontend-root', command: 'yarn nx run grafana:test:ci' },
  { name: 'test-frontend-packages', command: 'yarn run packages:test:ci' },
  { name: 'test-frontend-plugins', command: 'yarn run plugin:test:ci' },
  { name: 'knip', command: 'yarn nx run grafana:knip' },
].map((task) => ({ ...task, logFile: join(reportDir, `${task.name}.log`) }));

function streamLines(stream, target, prefix, writer) {
  const rl = readline.createInterface({ input: stream });
  rl.on('line', (line) => {
    writer.write(`${line}\n`);
    target.write(`[${prefix}] ${line}\n`);
  });

  return new Promise((resolve) => {
    rl.on('close', resolve);
  });
}

function readExcerpt(filePath, maxLength = 12000) {
  const contents = readFileSync(filePath, 'utf8');

  if (contents.length <= maxLength) {
    return contents;
  }

  return contents.slice(contents.length - maxLength);
}

function runTask(task) {
  return new Promise((resolve) => {
    const writer = createWriteStream(task.logFile, { flags: 'w' });
    const startedAt = Date.now();
    const child = spawn(task.command, {
      cwd: workspaceRoot,
      env: sharedEnv,
      shell: true,
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    const stdoutDone = streamLines(child.stdout, process.stdout, task.name, writer);
    const stderrDone = streamLines(child.stderr, process.stderr, task.name, writer);

    child.on('close', async (code, signal) => {
      await Promise.all([stdoutDone, stderrDone]);
      writer.end();

      resolve({
        ...task,
        durationMs: Date.now() - startedAt,
        exitCode: code,
        signal,
        status: code === 0 ? 'passed' : 'failed',
      });
    });
  });
}

const startedAt = Date.now();
const results = await Promise.all(tasks.map(runTask));
const failed = results.filter((result) => result.status === 'failed');

if (failed.length > 0) {
  const payload = {
    ok: false,
    reportDir,
    durationMs: Date.now() - startedAt,
    failedTasks: failed.length,
    tasks: results.map((result) => ({
      name: result.name,
      command: result.command,
      status: result.status,
      exitCode: result.exitCode,
      signal: result.signal,
      durationMs: result.durationMs,
      logFile: result.logFile,
      excerpt: result.status === 'failed' ? readExcerpt(result.logFile) : undefined,
    })),
  };

  process.stderr.write(`${JSON.stringify(payload, null, 2)}\n`);
  process.exit(1);
}

process.stdout.write(`ci-fast completed in ${Date.now() - startedAt}ms\n`);
for (const result of results) {
  process.stdout.write(`- ${result.name}: ${result.durationMs}ms\n`);
}
