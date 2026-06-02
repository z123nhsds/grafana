#!/usr/bin/env node

const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');

const CI_RESULTS_DIR = path.join(process.cwd(), 'ci-results');
if (!fs.existsSync(CI_RESULTS_DIR)) {
  fs.mkdirSync(CI_RESULTS_DIR, { recursive: true });
}

const tasks = {
  'lint-go': {
    command: 'make',
    args: ['lint-go'],
    outputFile: path.join(CI_RESULTS_DIR, 'lint-go.json'),
    parseOutput: (code, stdout, stderr) => ({
      success: code === 0,
      errors: stderr ? stderr.split('\n').filter(l => l.trim()) : [],
      output: stdout
    })
  },
  'lint-ts': {
    command: 'yarn',
    args: ['lint'],
    outputFile: path.join(CI_RESULTS_DIR, 'lint-ts.json'),
    parseOutput: (code, stdout, stderr) => ({
      success: code === 0,
      errors: stderr ? stderr.split('\n').filter(l => l.trim()) : [],
      output: stdout
    })
  },
  'typecheck': {
    command: 'yarn',
    args: ['typecheck'],
    outputFile: path.join(CI_RESULTS_DIR, 'typecheck.json'),
    parseOutput: (code, stdout, stderr) => ({
      success: code === 0,
      errors: stderr ? stderr.split('\n').filter(l => l.trim()) : [],
      output: stdout
    })
  },
  'test-go-unit': {
    command: 'make',
    args: ['test-go-unit'],
    outputFile: path.join(CI_RESULTS_DIR, 'test-go-unit.json'),
    parseOutput: (code, stdout, stderr) => ({
      success: code === 0,
      errors: stderr ? stderr.split('\n').filter(l => l.trim()) : [],
      output: stdout
    })
  },
  'test-ts': {
    command: 'yarn',
    args: ['test:ci'],
    outputFile: path.join(CI_RESULTS_DIR, 'test-ts.json'),
    parseOutput: (code, stdout, stderr) => ({
      success: code === 0,
      errors: stderr ? stderr.split('\n').filter(l => l.trim()) : [],
      output: stdout
    })
  },
  'knip': {
    command: 'yarn',
    args: ['knip'],
    outputFile: path.join(CI_RESULTS_DIR, 'knip.json'),
    parseOutput: (code, stdout, stderr) => ({
      success: code === 0,
      errors: stderr ? stderr.split('\n').filter(l => l.trim()) : [],
      output: stdout
    })
  }
};

function runTask(name, task) {
  return new Promise((resolve) => {
    console.log(`[${name}] Starting...`);
    const child = spawn(task.command, task.args, {
      cwd: process.cwd(),
      stdio: 'pipe',
      shell: true
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (data) => {
      stdout += data.toString();
      process.stdout.write(`[${name}] ${data.toString()}`);
    });

    child.stderr.on('data', (data) => {
      stderr += data.toString();
      process.stderr.write(`[${name}] ${data.toString()}`);
    });

    child.on('close', (code) => {
      const result = task.parseOutput(code, stdout, stderr);
      result.task = name;
      result.exitCode = code;
      fs.writeFileSync(task.outputFile, JSON.stringify(result, null, 2));
      console.log(`[${name}] Done. ${result.success ? '✅' : '❌'}`);
      resolve(result);
    });
  });
}

async function main() {
  console.log('🚀 Starting CI Fast Check...\n');

  const startTime = Date.now();
  const results = {};
  const promises = [];

  for (const [name, task] of Object.entries(tasks)) {
    promises.push(runTask(name, task).then(result => {
      results[name] = result;
    }));
  }

  await Promise.all(promises);

  const endTime = Date.now();
  const duration = (endTime - startTime) / 1000;

  const summary = {
    success: Object.values(results).every(r => r.success),
    durationSeconds: duration,
    results
  };

  const summaryFile = path.join(CI_RESULTS_DIR, 'summary.json');
  fs.writeFileSync(summaryFile, JSON.stringify(summary, null, 2));

  console.log('\n📊 CI Fast Check Summary:');
  console.log(`Duration: ${duration.toFixed(2)}s`);
  console.log(`Status: ${summary.success ? '✅ All passed' : '❌ Some failed'}`);
  console.log(`\nDetailed results saved to: ${CI_RESULTS_DIR}`);

  process.exit(summary.success ? 0 : 1);
}

main().catch(console.error);
