import { spawn } from 'child_process';

const tasks = [
  { name: 'lint-go', cmd: 'make', args: ['lint-go'] },
  { name: 'lint-ts', cmd: 'yarn', args: ['lint'] },
  { name: 'type-check', cmd: 'yarn', args: ['typecheck'] },
  { name: 'test-go', cmd: 'make', args: ['test-go-unit'] },
  { name: 'test-ts', cmd: 'yarn', args: ['test:ci'] },
  { name: 'knip', cmd: 'yarn', args: ['knip'] },
];

async function runTask(task) {
  return new Promise((resolve) => {
    const start = Date.now();
    const p = spawn(task.cmd, task.args, { shell: true });
    let stdout = '';
    let stderr = '';
    
    p.stdout.on('data', (data) => stdout += data.toString());
    p.stderr.on('data', (data) => stderr += data.toString());
    
    p.on('close', (code) => {
      resolve({
        name: task.name,
        code,
        durationMs: Date.now() - start,
        stdout: stdout.trim(),
        stderr: stderr.trim()
      });
    });
  });
}

async function main() {
  console.log(`Starting ci-fast with ${tasks.length} parallel tasks...`);
  const results = await Promise.all(tasks.map(runTask));
  const failures = results.filter(r => r.code !== 0);
  
  if (failures.length > 0) {
    const jsonOutput = {
      status: 'failed',
      totalTasks: tasks.length,
      failedCount: failures.length,
      failures: failures.map(f => ({
        task: f.name,
        exitCode: f.code,
        durationMs: f.durationMs,
        logs: (f.stdout + '\n' + f.stderr).substring(0, 5000)
      }))
    };
    console.error(JSON.stringify(jsonOutput, null, 2));
    process.exit(1);
  } else {
    console.log(`All ${tasks.length} tasks passed successfully.`);
    process.exit(0);
  }
}

main();
