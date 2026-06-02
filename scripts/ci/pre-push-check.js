#!/usr/bin/env node

const { execSync } = require('child_process');
const path = require('path');

function getChangedFiles() {
  try {
    const base = process.env.LEFTHOOK_BASE || 'main';
    const head = process.env.LEFTHOOK_HEAD || 'HEAD';
    const output = execSync(`git diff --name-only ${base}...${head}`, { encoding: 'utf-8' });
    return output.trim().split('\n').filter(f => f.trim());
  } catch (err) {
    console.warn('Warning: Could not get changed files, falling back to git status');
    const output = execSync('git status --porcelain', { encoding: 'utf-8' });
    return output.trim().split('\n').map(line => line.slice(3)).filter(f => f.trim());
  }
}

function detectChangedGoPackages(files) {
  const packages = new Set();
  for (const file of files) {
    if (file.endsWith('.go') && !file.endsWith('_test.go')) {
      const pkgDir = path.dirname(file);
      packages.add(pkgDir);
    }
  }
  return Array.from(packages);
}

function detectChangedTsPackages(files) {
  const packages = new Set();
  const workspacePatterns = [
    { pattern: /^packages\//, prefix: 'packages/' },
    { pattern: /^public\/app\/plugins\//, prefix: 'public/app/plugins/' },
    { pattern: /^e2e-playwright\/test-plugins\//, prefix: 'e2e-playwright/test-plugins/' }
  ];

  for (const file of files) {
    if (file.endsWith('.ts') || file.endsWith('.tsx') || file.endsWith('.js') || file.endsWith('.jsx')) {
      for (const wp of workspacePatterns) {
        if (wp.pattern.test(file)) {
          const parts = file.slice(wp.prefix.length).split('/');
          const pkgName = parts[0];
          packages.add(pkgName);
          break;
        }
      }
    }
  }
  return Array.from(packages);
}

function runGoTests(packages) {
  if (packages.length === 0) {
    console.log('📦 No Go packages changed, skipping Go tests');
    return true;
  }
  console.log(`🏃 Running tests for Go packages: ${packages.join(', ')}`);
  try {
    for (const pkg of packages) {
      execSync(`go test -v ./${pkg}`, { stdio: 'inherit' });
    }
    return true;
  } catch (err) {
    return false;
  }
}

function runTsTests(packages) {
  if (packages.length === 0) {
    console.log('📦 No TypeScript packages changed, skipping TS tests');
    return true;
  }
  console.log(`🏃 Running tests for TypeScript packages: ${packages.join(', ')}`);
  try {
    for (const pkg of packages) {
      if (pkg) {
        execSync(`yarn nx test ${pkg} --watch=false`, { stdio: 'inherit' });
      }
    }
    return true;
  } catch (err) {
    return false;
  }
}

async function main() {
  console.log('🔍 Pre-push check starting...\n');
  
  const changedFiles = getChangedFiles();
  if (changedFiles.length === 0) {
    console.log('✅ No changes detected, skipping tests');
    process.exit(0);
  }

  console.log(`📝 ${changedFiles.length} changed files detected`);

  const changedGoPackages = detectChangedGoPackages(changedFiles);
  const changedTsPackages = detectChangedTsPackages(changedFiles);

  const goSuccess = runGoTests(changedGoPackages);
  const tsSuccess = runTsTests(changedTsPackages);

  if (goSuccess && tsSuccess) {
    console.log('\n✅ All pre-push checks passed');
    process.exit(0);
  } else {
    console.log('\n❌ Pre-push checks failed');
    process.exit(1);
  }
}

main().catch(console.error);
