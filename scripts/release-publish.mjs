#!/usr/bin/env node

import { execFileSync, spawnSync } from 'node:child_process'
import fs from 'node:fs'
import https from 'node:https'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  ensureDockerWorkflowAvailable,
  verifyGhcrRelease,
  waitForDockerWorkflow,
  waitForDockerWorkflowStart,
} from './docker-release.mjs'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const rootDir = path.resolve(__dirname, '..')

const argv = process.argv.slice(2)
const push = argv.includes('--push')
const retag = argv.includes('--retag')
const verifyDocker = argv.includes('--verify-docker')
const allowOtherBranch = argv.includes('--allow-other-branch')
const remote = readOptionValue('--remote') || 'origin'
const branch = readOptionValue('--branch') || 'main'
const assetsDirOption = readOptionValue('--assets-dir') || 'releases'

const releaseRelevantRoots = [
  '.github',
  'api',
  'app',
  'cmd',
  'config',
  'core',
  'cronjob',
  'database',
  'docs',
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
  '.dockerignore',
  '.gitattributes',
  '.gitignore',
  'Dockerfile',
  'LICENSE',
  'README.md',
  'build.bat',
  'build.sh',
  'docker-compose.yml',
  'entrypoint.sh',
  'go.mod',
  'go.sum',
  'install.sh',
  'kwor.service',
  'main.go',
])

const requiredReleaseFiles = [
  'README.md',
]

const releaseIgnoredPrefixes = [
  'releases/',
  'temp_frontend/dist/',
  'web/html/',
]

if (argv.includes('--help') || argv.includes('-h')) {
  printHelp()
  process.exit(0)
}

if (verifyDocker && (push || retag)) {
  fail(['--verify-docker cannot be combined with --push or --retag.'])
}

process.chdir(rootDir)

runNodeScript(path.join('scripts', 'sync-version.mjs'), verifyDocker ? ['--check'] : [])

const version = readVersion()
const tagName = `v${version}`
const assetsDir = path.resolve(rootDir, assetsDirOption)

ensureGitRepository()
const headCommit = runGit(['rev-parse', 'HEAD']).trim()
const headSummary = runGit(['log', '-1', '--pretty=%h %s']).trim()

if (verifyDocker) {
  const repository = parseGitHubRepository(runGit(['remote', 'get-url', remote]).trim())
  const token = getGitHubToken()
  await runDockerStep('Docker workflow preflight failed', () => ensureDockerWorkflowAvailable({ repository, token }))
  const dockerRun = await runDockerStep('Docker workflow verification failed', () => waitForDockerWorkflow({
    repository,
    token,
    tagName,
    headCommit,
  }))
  const dockerRelease = await runDockerStep('GHCR verification failed', () => verifyGhcrRelease({
    repository,
    version,
  }))
  printDockerVerification(dockerRun, dockerRelease)
  process.exit(0)
}

ensureBranch(branch, allowOtherBranch)
ensureReleaseTreeCommitted()
ensureRequiredReleaseFilesCommitted()

const releaseAssets = push ? readReleaseAssets(assetsDir, version) : []
let repository = null
let token = ''

if (push) {
  repository = parseGitHubRepository(runGit(['remote', 'get-url', remote]).trim())
  token = requireGitHubToken()
  await runDockerStep('Docker workflow preflight failed', () => ensureDockerWorkflowAvailable({ repository, token }))
  await ensureReleaseDoesNotExist({ repository, token, tagName })
}

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
  console.log(`Push, verify Docker, and upload local assets with: node scripts/release-publish.mjs --push --assets-dir "${assetsDir}"${retag ? ' --retag' : ''}`)
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

await createAndUploadRelease({
  repository,
  token,
  tagName,
  branch,
  releaseAssets,
})

const dockerRun = await runDockerStep('Docker workflow did not start', () => waitForDockerWorkflowStart({
  repository,
  token,
  tagName,
  headCommit,
}))

console.log(`GitHub Release ${tagName} published from ${headSummary}`)
console.log(`Docker workflow started: ${dockerRun.html_url || 'GitHub Actions run URL unavailable'}`)

function printHelp() {
  console.log(`Usage: node scripts/release-publish.mjs [--push] [--assets-dir <directory>] [--retag] [--verify-docker] [--remote origin] [--branch main]

This helper publishes the current committed source tree without generating history comparisons:
1. syncs package, Docker Compose, and README version metadata from config/version
2. checks that release-source changes are committed
3. creates tag v<config/version> at current HEAD
4. pushes the branch and tag
5. creates a draft GitHub Release, uploads and verifies six local assets, then publishes it
6. confirms that the published Release started Docker publication without waiting for completion

Options:
  --push                    push HEAD and tag, publish the GitHub Release, then confirm Docker publication starts
  --assets-dir <directory>  directory containing the six local release assets (default: releases)
  --retag                   move an existing v<version> tag before its GitHub Release exists
  --verify-docker           verify the current version/HEAD Docker workflow and GHCR image without publishing
  --remote <name>           git remote to push to (default: origin)
  --branch <name>           branch to push HEAD to (default: main)
  --allow-other-branch      skip the current-branch check
`)
}

function readOptionValue(name) {
  const index = argv.indexOf(name)
  if (index === -1) {
    return ''
  }
  return argv[index + 1] || ''
}

function runNodeScript(relativePath, args = []) {
  const result = spawnSync(process.execPath, [relativePath, ...args], {
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
  if (!/^\d+\.\d+\.\d+(?:[-._A-Za-z0-9]+)?$/.test(version)) {
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
    'Commit or discard these files before tagging so the pushed source matches the local release assets.',
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

function ensureRequiredReleaseFilesCommitted() {
  const missing = requiredReleaseFiles.filter(filePath => {
    try {
      runGit(['cat-file', '-e', `HEAD:${filePath}`])
      return false
    } catch {
      return true
    }
  })

  if (missing.length === 0) {
    return
  }

  fail([
    'Release aborted: required files are missing from current HEAD.',
    'Commit these files before tagging so the source tree and repository homepage remain complete.',
    ...missing.map(filePath => `  - ${filePath}`),
  ])
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

function readReleaseAssets(directory, version) {
  const names = [
    'kwor_amd64',
    'kwor_arm64',
    'kwor-linux-amd64.tar.gz',
    'kwor-linux-arm64.tar.gz',
    `kwor-v${version}-source.tar.gz`,
    `kwor-v${version}-source.zip`,
  ]

  const assets = names.map(name => {
    const filePath = path.join(directory, name)
    if (!fs.existsSync(filePath)) {
      fail([`Missing release asset: ${filePath}`])
    }
    const stat = fs.statSync(filePath)
    if (!stat.isFile() || stat.size === 0) {
      fail([`Invalid release asset: ${filePath}`])
    }
    return { name, filePath, size: stat.size }
  })

  console.log(`Validated ${assets.length} local release assets from ${directory}`)
  return assets
}

function resolveTagCommit(tagName) {
  try {
    return runGit(['rev-list', '-n', '1', tagName]).trim()
  } catch {
    return ''
  }
}

function parseGitHubRepository(remoteUrl) {
  const normalized = remoteUrl.trim().replace(/\.git$/, '')
  const match = normalized.match(/github\.com[/:]([^/:\s]+)\/([^/\s]+)$/i)
  if (!match) {
    fail([`Remote ${remoteUrl} is not a GitHub repository URL.`])
  }
  return { owner: match[1], repo: match[2] }
}

function getGitHubToken() {
  for (const name of ['GH_TOKEN', 'GITHUB_TOKEN', 'GITHUB_PAT']) {
    const token = process.env[name]?.trim()
    if (token) {
      return token
    }
  }

  const result = spawnSync('git', ['-c', 'credential.interactive=false', 'credential', 'fill'], {
    cwd: rootDir,
    input: 'protocol=https\nhost=github.com\n\n',
    encoding: 'utf8',
    env: {
      ...process.env,
      GIT_TERMINAL_PROMPT: '0',
    },
  })
  if (result.status !== 0) {
    return ''
  }

  const passwordLine = String(result.stdout || '')
    .split(/\r?\n/)
    .find(line => line.startsWith('password='))
  return passwordLine ? passwordLine.slice('password='.length).trim() : ''
}

function requireGitHubToken() {
  const token = getGitHubToken()
  if (!token) {
    fail([
      'No GitHub API credential is available.',
      'Set GH_TOKEN or GITHUB_TOKEN, or sign in through Git Credential Manager and run this command again.',
    ])
  }
  return token
}

async function ensureReleaseDoesNotExist({ repository, token, tagName }) {
  const response = await githubRequest({
    method: 'GET',
    hostname: 'api.github.com',
    requestPath: `/repos/${encodeURIComponent(repository.owner)}/${encodeURIComponent(repository.repo)}/releases/tags/${encodeURIComponent(tagName)}`,
    token,
  })
  if (response.statusCode === 200) {
    fail([`GitHub Release ${tagName} already exists; refusing to move its tag or overwrite it.`])
  }
  if (response.statusCode !== 404) {
    fail([`Cannot check GitHub Release ${tagName}: ${formatGitHubError(response)}`])
  }
}

async function runDockerStep(label, operation) {
  try {
    return await operation()
  } catch (error) {
    fail([`${label}: ${error instanceof Error ? error.message : String(error)}`])
  }
}

function printDockerVerification(run, release) {
  console.log(`Docker workflow succeeded: ${run.html_url}`)
  console.log(`Verified ${release.image} tags ${release.tags.join(', ')} at ${release.digest}`)
  console.log(`Verified Docker platforms: ${release.platforms.join(', ')}`)
}

async function createAndUploadRelease({ repository, token, tagName, branch, releaseAssets }) {
  const releasePath = `/repos/${encodeURIComponent(repository.owner)}/${encodeURIComponent(repository.repo)}/releases/tags/${encodeURIComponent(tagName)}`
  const existing = await githubRequest({
    method: 'GET',
    hostname: 'api.github.com',
    requestPath: releasePath,
    token,
  })
  if (existing.statusCode === 200) {
    fail([`GitHub Release ${tagName} already exists; refusing to overwrite it.`])
  }
  if (existing.statusCode !== 404) {
    fail([`Cannot check GitHub Release ${tagName}: ${formatGitHubError(existing)}`])
  }

  const created = await githubRequest({
    method: 'POST',
    hostname: 'api.github.com',
    requestPath: `/repos/${encodeURIComponent(repository.owner)}/${encodeURIComponent(repository.repo)}/releases`,
    token,
    body: {
      tag_name: tagName,
      target_commitish: branch,
      name: `kwor ${tagName}`,
      body: '',
      draft: true,
      prerelease: false,
      generate_release_notes: false,
    },
  })
  if (created.statusCode !== 201 || !created.data?.id) {
    fail([`Create GitHub Release ${tagName} failed: ${formatGitHubError(created)}`])
  }

  console.log(`Created GitHub Release draft ${tagName}; uploading local assets...`)
  for (const asset of releaseAssets) {
    const uploaded = await uploadReleaseAsset({
      repository,
      token,
      releaseId: created.data.id,
      asset,
    })
    if (uploaded.statusCode !== 201) {
      fail([`Upload ${asset.name} failed: ${formatGitHubError(uploaded)}`])
    }
    console.log(`Uploaded ${asset.name}`)
  }

  const draftReleasePath = `/repos/${encodeURIComponent(repository.owner)}/${encodeURIComponent(repository.repo)}/releases/${created.data.id}`
  const draftRelease = await githubRequest({
    method: 'GET',
    hostname: 'api.github.com',
    requestPath: draftReleasePath,
    token,
  })
  if (draftRelease.statusCode !== 200) {
    fail([`GitHub Release draft ${tagName} verification failed: ${formatGitHubError(draftRelease)}`])
  }
  verifyReleaseAssets({ release: draftRelease.data, tagName, releaseAssets, expectDraft: true })

  const published = await githubRequest({
    method: 'PATCH',
    hostname: 'api.github.com',
    requestPath: draftReleasePath,
    token,
    body: { draft: false },
  })
  if (published.statusCode !== 200) {
    fail([`Publish GitHub Release ${tagName} failed: ${formatGitHubError(published)}`])
  }

  const finalRelease = await githubRequest({
    method: 'GET',
    hostname: 'api.github.com',
    requestPath: draftReleasePath,
    token,
  })
  if (finalRelease.statusCode !== 200) {
    fail([`Published GitHub Release ${tagName} verification failed: ${formatGitHubError(finalRelease)}`])
  }
  verifyReleaseAssets({ release: finalRelease.data, tagName, releaseAssets, expectDraft: false })
  console.log(`Published GitHub Release ${tagName}`)
}

function verifyReleaseAssets({ release, tagName, releaseAssets, expectDraft }) {
  const uploadedNames = new Set((release?.assets || []).map(asset => asset.name))
  const missing = releaseAssets.map(asset => asset.name).filter(name => !uploadedNames.has(name))
  const unexpected = [...uploadedNames].filter(name => !releaseAssets.some(asset => asset.name === name))
  const errors = [
    release?.draft !== expectDraft ? `Release draft state is ${String(release?.draft)}, expected ${String(expectDraft)}.` : '',
    !expectDraft && !String(release?.published_at ?? '').trim() ? 'Release was not published.' : '',
    release?.prerelease === true ? 'Release is marked as prerelease.' : '',
    missing.length > 0 ? `Missing assets: ${missing.join(', ')}` : '',
    unexpected.length > 0 ? `Unexpected assets: ${unexpected.join(', ')}` : '',
    String(release?.body ?? '') !== '' ? 'Release body is not empty.' : '',
  ].filter(Boolean)
  if (errors.length > 0) {
    fail([`GitHub Release ${tagName} verification failed.`, ...errors])
  }
}

function githubRequest({ method, hostname, requestPath, token, body }) {
  const payload = body === undefined ? null : Buffer.from(JSON.stringify(body))
  return new Promise((resolve, reject) => {
    const request = https.request({
      hostname,
      method,
      path: requestPath,
      headers: {
        Accept: 'application/vnd.github+json',
        Authorization: `Bearer ${token}`,
        'User-Agent': 'kwor-release-publish',
        'X-GitHub-Api-Version': '2022-11-28',
        ...(payload ? {
          'Content-Type': 'application/json',
          'Content-Length': payload.length,
        } : {}),
      },
    }, response => {
      const chunks = []
      response.on('data', chunk => chunks.push(chunk))
      response.on('end', () => {
        const text = Buffer.concat(chunks).toString('utf8')
        let data = null
        try {
          data = text ? JSON.parse(text) : null
        } catch {
          data = null
        }
        resolve({ statusCode: response.statusCode ?? 0, data, text })
      })
    })
    request.on('error', reject)
    if (payload) {
      request.write(payload)
    }
    request.end()
  })
}

function uploadReleaseAsset({ repository, token, releaseId, asset }) {
  return new Promise((resolve, reject) => {
    const requestPath = `/repos/${encodeURIComponent(repository.owner)}/${encodeURIComponent(repository.repo)}/releases/${releaseId}/assets?name=${encodeURIComponent(asset.name)}`
    const request = https.request({
      hostname: 'uploads.github.com',
      method: 'POST',
      path: requestPath,
      headers: {
        Accept: 'application/vnd.github+json',
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/octet-stream',
        'Content-Length': asset.size,
        'User-Agent': 'kwor-release-publish',
        'X-GitHub-Api-Version': '2022-11-28',
      },
    }, response => {
      const chunks = []
      response.on('data', chunk => chunks.push(chunk))
      response.on('end', () => {
        const text = Buffer.concat(chunks).toString('utf8')
        let data = null
        try {
          data = text ? JSON.parse(text) : null
        } catch {
          data = null
        }
        resolve({ statusCode: response.statusCode ?? 0, data, text })
      })
    })
    request.on('error', reject)

    const input = fs.createReadStream(asset.filePath)
    input.on('error', reject)
    input.pipe(request)
  })
}

function formatGitHubError(response) {
  return response.data?.message || response.text || `HTTP ${response.statusCode}`
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
