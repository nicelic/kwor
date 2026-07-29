import assert from 'node:assert/strict'
import test from 'node:test'

import {
  expectedDockerTags,
  selectDockerWorkflowRun,
  validateDockerManifestSet,
} from './docker-release.mjs'

test('expectedDockerTags publishes latest only for stable versions', () => {
  assert.deepEqual(expectedDockerTags('1.6.5'), ['v1.6.5', '1.6.5', 'latest'])
  assert.deepEqual(expectedDockerTags('1.7.0-rc.1'), ['v1.7.0-rc.1', '1.7.0-rc.1'])
})

test('selectDockerWorkflowRun matches the exact release tag and commit', () => {
  const selected = selectDockerWorkflowRun([
    { id: 1, event: 'release', head_branch: 'v1.6.5', head_sha: 'old' },
    { id: 2, event: 'workflow_dispatch', head_branch: 'v1.6.5', head_sha: 'wanted' },
    { id: 3, event: 'release', head_branch: 'v1.6.5', head_sha: 'wanted' },
    { id: 4, event: 'release', head_branch: 'v1.6.4', head_sha: 'wanted' },
  ], {
    tagName: 'v1.6.5',
    headCommit: 'wanted',
  })

  assert.equal(selected?.id, 3)
})

test('validateDockerManifestSet accepts both platforms and ignores attestations', () => {
  const manifests = ['v1.6.5', '1.6.5', 'latest'].map(tag => createManifest(tag, 'sha256:same', [
    ['linux', 'amd64'],
    ['linux', 'arm64'],
    ['unknown', 'unknown'],
  ]))

  assert.deepEqual(validateDockerManifestSet({ version: '1.6.5', manifests }), {
    tags: ['v1.6.5', '1.6.5', 'latest'],
    digest: 'sha256:same',
    platforms: ['linux/amd64', 'linux/arm64'],
  })
})

test('validateDockerManifestSet rejects digest mismatches', () => {
  const manifests = [
    createManifest('v1.6.5', 'sha256:first'),
    createManifest('1.6.5', 'sha256:second'),
    createManifest('latest', 'sha256:first'),
  ]

  assert.throws(
    () => validateDockerManifestSet({ version: '1.6.5', manifests }),
    /do not point to one manifest digest/,
  )
})

test('validateDockerManifestSet rejects a missing target platform', () => {
  const manifests = ['v1.6.5', '1.6.5', 'latest'].map(tag => createManifest(tag, 'sha256:same', [
    ['linux', 'amd64'],
  ]))

  assert.throws(
    () => validateDockerManifestSet({ version: '1.6.5', manifests }),
    /linux\/arm64/,
  )
})

function createManifest(tag, digest, platforms = [['linux', 'amd64'], ['linux', 'arm64']]) {
  return {
    tag,
    statusCode: 200,
    headers: {
      'docker-content-digest': digest,
    },
    data: {
      manifests: platforms.map(([os, architecture]) => ({
        platform: { os, architecture },
      })),
    },
  }
}
