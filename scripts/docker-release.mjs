import https from 'node:https'

export const DOCKER_WORKFLOW_FILE = 'docker.yml'
export const DOCKER_WORKFLOW_TIMEOUT_MS = 60 * 60 * 1000
export const DOCKER_WORKFLOW_START_TIMEOUT_MS = 2 * 60 * 1000
export const DOCKER_REGISTRY_TIMEOUT_MS = 5 * 60 * 1000

const dockerWorkflowPollIntervalMs = 20 * 1000
const dockerWorkflowStartPollIntervalMs = 5 * 1000
const dockerRegistryPollIntervalMs = 10 * 1000
const requiredPlatforms = ['linux/amd64', 'linux/arm64']
const manifestAccept = [
  'application/vnd.oci.image.index.v1+json',
  'application/vnd.docker.distribution.manifest.list.v2+json',
  'application/vnd.oci.image.manifest.v1+json',
  'application/vnd.docker.distribution.manifest.v2+json',
].join(', ')

export function isStableVersion(version) {
  return /^\d+\.\d+\.\d+$/.test(version)
}

export function expectedDockerTags(version) {
  const tags = [`v${version}`, version]
  if (isStableVersion(version)) {
    tags.push('latest')
  }
  return tags
}

export function selectDockerWorkflowRun(runs, { tagName, headCommit }) {
  return [...runs]
    .filter(run => run?.event === 'release')
    .filter(run => run?.head_sha === headCommit)
    .sort((left, right) => {
      const tagMatch = Number(String(right?.head_branch || '') === tagName) - Number(String(left?.head_branch || '') === tagName)
      return tagMatch || Number(right.id || 0) - Number(left.id || 0)
    })[0] || null
}

export function validateDockerManifestSet({ version, manifests }) {
  const expectedTags = expectedDockerTags(version)
  const byTag = new Map(manifests.map(manifest => [manifest.tag, manifest]))
  const digests = []

  for (const tag of expectedTags) {
    const manifest = byTag.get(tag)
    if (!manifest || manifest.statusCode !== 200) {
      throw new Error(`GHCR manifest ${tag} is unavailable (HTTP ${manifest?.statusCode || 0})`)
    }

    const digest = String(manifest.headers?.['docker-content-digest'] || '').trim()
    if (!digest) {
      throw new Error(`GHCR manifest ${tag} did not return Docker-Content-Digest`)
    }
    digests.push(digest)

    const platforms = new Set(
      (manifest.data?.manifests || []).map(item => `${item?.platform?.os || ''}/${item?.platform?.architecture || ''}`),
    )
    const missingPlatforms = requiredPlatforms.filter(platform => !platforms.has(platform))
    if (missingPlatforms.length > 0) {
      throw new Error(`GHCR manifest ${tag} is missing platforms: ${missingPlatforms.join(', ')}`)
    }
  }

  const uniqueDigests = [...new Set(digests)]
  if (uniqueDigests.length !== 1) {
    throw new Error(`GHCR tags do not point to one manifest digest: ${uniqueDigests.join(', ')}`)
  }

  return {
    tags: expectedTags,
    digest: uniqueDigests[0],
    platforms: [...requiredPlatforms],
  }
}

export async function ensureDockerWorkflowAvailable({ repository, token }) {
  const response = await githubRequest({
    token,
    requestPath: `/repos/${encodeURIComponent(repository.owner)}/${encodeURIComponent(repository.repo)}/actions/workflows/${encodeURIComponent(DOCKER_WORKFLOW_FILE)}`,
  })
  if (response.statusCode !== 200) {
    throw new Error(`Cannot access Docker workflow ${DOCKER_WORKFLOW_FILE}: ${formatResponseError(response)}`)
  }
  if (response.data?.state !== 'active') {
    throw new Error(`Docker workflow ${DOCKER_WORKFLOW_FILE} is not active (state: ${response.data?.state || 'unknown'})`)
  }
  return response.data
}

export async function waitForDockerWorkflow({
  repository,
  token,
  tagName,
  headCommit,
  timeoutMs = DOCKER_WORKFLOW_TIMEOUT_MS,
  pollIntervalMs = dockerWorkflowPollIntervalMs,
  log = console.log,
}) {
  return waitForDockerWorkflowRun({
    repository,
    token,
    tagName,
    headCommit,
    timeoutMs,
    pollIntervalMs,
    waitForCompletion: true,
    log,
  })
}

export async function waitForDockerWorkflowStart({
  repository,
  token,
  tagName,
  headCommit,
  timeoutMs = DOCKER_WORKFLOW_START_TIMEOUT_MS,
  pollIntervalMs = dockerWorkflowStartPollIntervalMs,
  log = console.log,
}) {
  return waitForDockerWorkflowRun({
    repository,
    token,
    tagName,
    headCommit,
    timeoutMs,
    pollIntervalMs,
    waitForCompletion: false,
    log,
  })
}

async function waitForDockerWorkflowRun({
  repository,
  token,
  tagName,
  headCommit,
  timeoutMs,
  pollIntervalMs,
  waitForCompletion,
  log,
}) {
  const deadline = Date.now() + timeoutMs
  const effectivePollIntervalMs = token ? pollIntervalMs : Math.max(pollIntervalMs, 60 * 1000)
  let lastStatus = ''

  while (Date.now() <= deadline) {
    const query = new URLSearchParams({
      event: 'release',
      per_page: '20',
    })
    const response = await githubRequest({
      token,
      requestPath: `/repos/${encodeURIComponent(repository.owner)}/${encodeURIComponent(repository.repo)}/actions/workflows/${encodeURIComponent(DOCKER_WORKFLOW_FILE)}/runs?${query}`,
    })
    if (response.statusCode !== 200) {
      throw new Error(`Cannot read Docker workflow runs: ${formatResponseError(response)}`)
    }

    const run = selectDockerWorkflowRun(response.data?.workflow_runs || [], { tagName, headCommit })
    if (!run) {
      if (lastStatus !== 'not-found') {
        log(`Waiting for Docker workflow ${DOCKER_WORKFLOW_FILE} to start for ${tagName} ...`)
        lastStatus = 'not-found'
      }
    } else {
      const currentStatus = `${run.status || 'unknown'}/${run.conclusion || 'pending'}`
      if (currentStatus !== lastStatus) {
        log(`Docker workflow ${run.status || 'unknown'} for ${tagName}: ${run.html_url || ''}`)
        lastStatus = currentStatus
      }
      if (!waitForCompletion) {
        return run
      }
      if (run.status === 'completed') {
        if (run.conclusion !== 'success') {
          throw new Error(`Docker workflow failed with conclusion ${run.conclusion || 'unknown'}: ${run.html_url || ''}`)
        }
        return run
      }
    }

    await delay(Math.min(effectivePollIntervalMs, Math.max(0, deadline - Date.now())))
  }

  const phase = waitForCompletion ? 'complete' : 'start'
  throw new Error(`Timed out after ${Math.round(timeoutMs / 60000)} minutes waiting for Docker workflow ${tagName} to ${phase}`)
}

export async function verifyGhcrRelease({
  repository,
  version,
  timeoutMs = DOCKER_REGISTRY_TIMEOUT_MS,
  pollIntervalMs = dockerRegistryPollIntervalMs,
  log = console.log,
}) {
  const image = `${repository.owner}/${repository.repo}`.toLowerCase()
  const tags = expectedDockerTags(version)
  const deadline = Date.now() + timeoutMs
  let token = await getGhcrPullToken(image)
  let lastErrorMessage = ''

  while (Date.now() <= deadline) {
    const manifests = await Promise.all(tags.map(tag => readGhcrManifest({ image, tag, token })))
    try {
      return {
        image: `ghcr.io/${image}`,
        ...validateDockerManifestSet({ version, manifests }),
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      if (message !== lastErrorMessage) {
        log(`Waiting for GHCR manifests: ${message}`)
        lastErrorMessage = message
      }
      if (manifests.some(manifest => manifest.statusCode === 401)) {
        token = await getGhcrPullToken(image)
      }
    }

    await delay(Math.min(pollIntervalMs, Math.max(0, deadline - Date.now())))
  }

  throw new Error(`GHCR verification timed out for ghcr.io/${image}: ${lastErrorMessage || 'manifest unavailable'}`)
}

async function getGhcrPullToken(image) {
  const query = new URLSearchParams({
    service: 'ghcr.io',
    scope: `repository:${image}:pull`,
  })
  const response = await requestJson({
    hostname: 'ghcr.io',
    requestPath: `/token?${query}`,
    headers: {
      'User-Agent': 'kwor-release-publish',
    },
  })
  const token = response.data?.token || response.data?.access_token
  if (response.statusCode !== 200 || !token) {
    throw new Error(`Cannot obtain GHCR pull token: ${formatResponseError(response)}`)
  }
  return token
}

async function readGhcrManifest({ image, tag, token }) {
  const response = await requestJson({
    hostname: 'ghcr.io',
    requestPath: `/v2/${image}/manifests/${encodeURIComponent(tag)}`,
    headers: {
      Accept: manifestAccept,
      Authorization: `Bearer ${token}`,
      'User-Agent': 'kwor-release-publish',
    },
  })
  return { tag, ...response }
}

function githubRequest({ token, requestPath }) {
  return requestJson({
    hostname: 'api.github.com',
    requestPath,
    headers: {
      Accept: 'application/vnd.github+json',
      'User-Agent': 'kwor-release-publish',
      'X-GitHub-Api-Version': '2022-11-28',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  })
}

function requestJson({ hostname, requestPath, headers }) {
  return new Promise((resolve, reject) => {
    const request = https.request({
      hostname,
      method: 'GET',
      path: requestPath,
      headers,
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
        const normalizedHeaders = Object.fromEntries(
          Object.entries(response.headers).map(([name, value]) => [
            name.toLowerCase(),
            Array.isArray(value) ? value.join(', ') : String(value || ''),
          ]),
        )
        resolve({
          statusCode: response.statusCode || 0,
          headers: normalizedHeaders,
          data,
          text,
        })
      })
    })
    request.on('error', reject)
    request.end()
  })
}

function formatResponseError(response) {
  return response.data?.message || response.text || `HTTP ${response.statusCode}`
}

function delay(milliseconds) {
  if (milliseconds <= 0) {
    return Promise.resolve()
  }
  return new Promise(resolve => setTimeout(resolve, milliseconds))
}
