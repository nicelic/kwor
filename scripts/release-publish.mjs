#!/usr/bin/env node

import { execFileSync, spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const rootDir = path.resolve(__dirname, '..')

const argv = process.argv.slice(2)
const push = argv.includes('--push')
const retag = argv.includes('--retag')
const allowOtherBranch = argv.includes('--allow-other-branch')
const remote = readOptionValue('--remote') || 'origin'
const branch = readOptionValue('--branch') || 'main'

const releaseRelevantRoots = [
  '.github',
  'api',
  'app',
  'cmd',
  'config',
  'core',
  'cronjob',
  'database',
  'logger',
  'middleware',
  'network',
  'scripts',
  'service',
  'sub',
  'temp_frontend',
  'util',
  'web',
  'windows',
]

const releaseRelevantFiles = new Set([
  'Dockerfile',
  'README.md',
  'User Manual.md',
  'build.bat',
  'build.sh',
  'docker-compose.yml',
  'entrypoint.sh',
  'go.mod',
  'go.sum',
  'install.sh',
  'kwor.service',
  'main.go',
  '使用手册.md',
])

const releaseIgnoredPrefixes = [
  'releases/',
  'temp_frontend/dist/',
  'web/html/',
]

if (argv.includes('--help') || argv.includes('-h')) {
  printHelp()
  process.exit(0)
}

process.chdir(rootDir)

runNodeScript(path.join('scripts', 'sync-version.mjs'))

const version = readVersion()
const tagName = `v${version}`

ensureGitRepository()
ensureBranch(branch, allowOtherBranch)
ensureReleaseTreeCommitted()

const headCommit = runGit(['rev-parse', 'HEAD']).trim()
const headSummary = runGit(['log', '-1', '--pretty=%h %s']).trim()
const tagCommit = resolveTagCommit(tagName)

if (!tagCommit) {
  runGit(['tag', '-a', tagName, '-m', `release ${tagName}`], { stdio: 'inherit' })
  console.log(`Created local tag ${tagName} at ${headSummary}`)
} else if (tagCommit === headCommit) {
  console.log(`Local tag ${tagName} already points to ${headSummary}`)
} else if (!retag) {
  fail([
    `${tagName} already points to ${tagCommit.slice(0, 12)}, not current HEAD ${headCommit.slice(0, 12)}.`,
    'Use --retag only when you intentionally want to move the existing version tag to the current commit.',
  ])
} else {
  runGit(['tag', '-fa', tagName, '-m', `release ${tagName}`], { stdio: 'inherit' })
  console.log(`Moved local tag ${tagName} to ${headSummary}`)
}

if (!push) {
  console.log(`Release source is ready: ${headSummary}`)
  console.log(`Push with: node scripts/release-publish.mjs --push${retag ? ' --retag' : ''}`)
  process.exit(0)
}

console.log(`Pushing ${headSummary} to ${remote}/${branch} ...`)
runGit(['push', remote, `HEAD:${branch}`], { stdio: 'inherit' })

if (retag) {
  console.log(`Force-pushing tag ${tagName} to ${remote} ...`)
  runGit(['push', '--force', remote, `refs/tags/${tagName}`], { stdio: 'inherit' })
} else {
  console.log(`Pushing tag ${tagName} to ${remote} ...`)
  runGit(['push', remote, `refs/tags/${tagName}`], { stdio: 'inherit' })
}

console.log(`Release push completed for ${tagName} from ${headSummary}`)

function printHelp() {
  console.log(`Usage: node scripts/release-publish.mjs [--push] [--retag] [--remote origin] [--branch main]

This helper publishes the current committed source tree without comparing history:
1. syncs frontend version metadata from config/version
2. blocks release if local release source changes are still uncommitted
3. creates tag v<config/version> at current HEAD
4. optionally pushes the branch and tag to GitHub

Options:
  --push                push HEAD to the target branch and push the release tag
  --retag               move an existing v<version> tag to current HEAD
  --remote <name>       git remote to push to (default: origin)
  --branch <name>       branch to push HEAD to (default: main)
  --allow-other-branch  skip the current-branch check
`)
}

function readOptionValue(name) {
  const index = argv.indexOf(name)
  if (index === -1) {
    return ''
  }
  return argv[index + 1] || ''
}

function runNodeScript(relativePath) {
  const result = spawnSync(process.execPath, [relativePath], {
    cwd: rootDir,
    stdio: 'inherit',
  })
  if (result.status !== 0) {
    process.exit(result.status ?? 1)
  }
}

function readVersion() {
  const versionPath = path.join(rootDir, 'config', 'version')
  const version = fs.readFileSync(versionPath, 'utf8').trim()
  if (!/^\d+\.\d+\.\d+(?:[-+._A-Za-z0-9]+)?$/.test(version)) {
    fail([`Invalid version in config/version: ${version}`])
  }
  return version
}

function ensureGitRepository() {
  runGit(['rev-parse', '--show-toplevel'])
}

function ensureBranch(expectedBranch, allowOther) {
  const currentBranch = runGit(['branch', '--show-current']).trim()
  if (!allowOther && currentBranch !== expectedBranch) {
    fail([
      `Current branch is ${currentBranch || '(detached HEAD)'}, expected ${expectedBranch}.`,
      'Switch to the release branch first, or pass --allow-other-branch if this is intentional.',
    ])
  }
}

function ensureReleaseTreeCommitted() {
  const tracked = uniqueSorted([
    ...splitLines(runGit(['diff', '--name-only', '--'])),
    ...splitLines(runGit(['diff', '--cached', '--name-only', '--'])),
  ]).filter(isReleaseRelevantPath)

  const untracked = uniqueSorted(
    splitLines(runGit(['ls-files', '--others', '--exclude-standard'])),
  ).filter(isReleaseRelevantPath)

  if (tracked.length === 0 && untracked.length === 0) {
    return
  }

  const lines = [
    'Release aborted: current HEAD does not include all local release-source changes.',
    'Commit or discard these files before tagging, otherwise GitHub Release will build from an older commit.',
  ]

  if (tracked.length > 0) {
    lines.push('Tracked changes:')
    for (const filePath of tracked) {
      lines.push(`  - ${filePath}`)
    }
  }

  if (untracked.length > 0) {
    lines.push('Untracked release files:')
    for (const filePath of untracked) {
      lines.push(`  - ${filePath}`)
    }
  }

  fail(lines)
}

function isReleaseRelevantPath(filePath) {
  const normalized = normalizeRepoPath(filePath)
  if (!normalized) {
    return false
  }
  if (releaseIgnoredPrefixes.some(prefix => normalized === prefix.slice(0, -1) || normalized.startsWith(prefix))) {
    return false
  }
  if (releaseRelevantFiles.has(normalized)) {
    return true
  }
  return releaseRelevantRoots.some(root => normalized === root || normalized.startsWith(`${root}/`))
}

function resolveTagCommit(tagName) {
  try {
    return runGit(['rev-list', '-n', '1', tagName]).trim()
  } catch {
    return ''
  }
}

function runGit(args, options = {}) {
  return execFileSync('git', args, {
    cwd: rootDir,
    encoding: 'utf8',
    stdio: options.stdio || 'pipe',
  })
}

function splitLines(raw) {
  return raw
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(Boolean)
}

function uniqueSorted(values) {
  return [...new Set(values)].sort((left, right) => left.localeCompare(right))
}

function normalizeRepoPath(filePath) {
  return filePath.replace(/\\/g, '/').replace(/^\.\//, '').trim()
}

function fail(lines) {
  for (const line of lines) {
    console.error(line)
  }
  process.exit(1)
}
