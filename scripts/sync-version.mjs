import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const rootDir = path.resolve(__dirname, '..')
const checkOnly = process.argv.includes('--check')

const versionFile = path.join(rootDir, 'config', 'version')
const packageJsonFile = path.join(rootDir, 'temp_frontend', 'package.json')
const packageLockFile = path.join(rootDir, 'temp_frontend', 'package-lock.json')
const dockerComposeFile = path.join(rootDir, 'docker-compose.yml')
const readmeFile = path.join(rootDir, 'README.md')

const version = fs.readFileSync(versionFile, 'utf8').trim()

if (!/^\d+\.\d+\.\d+(?:[-._A-Za-z0-9]+)?$/.test(version)) {
  throw new Error(`Invalid version in ${versionFile}: ${version}`)
}

const updateJsonFile = (filePath, updater) => {
  const original = fs.readFileSync(filePath, 'utf8')
  const parsed = JSON.parse(original)
  const changed = updater(parsed)

  if (!changed) {
    return false
  }

  if (!checkOnly) {
    fs.writeFileSync(filePath, `${JSON.stringify(parsed, null, 2)}\n`, 'utf8')
  }
  return true
}

const updateTextFile = (filePath, pattern, replacement) => {
  const original = fs.readFileSync(filePath, 'utf8')
  const flags = pattern.flags.includes('g') ? pattern.flags : `${pattern.flags}g`
  const matches = [...original.matchAll(new RegExp(pattern.source, flags))]
  if (matches.length !== 1) {
    throw new Error(`Expected exactly one version reference in ${filePath}, found ${matches.length}`)
  }

  const updated = original.replace(pattern, replacement)
  if (updated === original) {
    return false
  }

  if (!checkOnly) {
    fs.writeFileSync(filePath, updated, 'utf8')
  }
  return true
}

const packageJsonChanged = updateJsonFile(packageJsonFile, data => {
  if (data.version === version) {
    return false
  }
  data.version = version
  return true
})

const packageLockChanged = updateJsonFile(packageLockFile, data => {
  let changed = false

  if (data.version !== version) {
    data.version = version
    changed = true
  }

  if (data.packages?.['']?.version !== version) {
    data.packages = data.packages ?? {}
    data.packages[''] = data.packages[''] ?? {}
    data.packages[''].version = version
    changed = true
  }

  return changed
})

const dockerComposeChanged = updateTextFile(
  dockerComposeFile,
  /^(\s*image:\s*ghcr\.io\/nicelic\/kwor:)v[^\s#]+/m,
  `$1v${version}`,
)

const readmeChanged = updateTextFile(
  readmeFile,
  /(ghcr\.io\/nicelic\/kwor:)v\d+\.\d+\.\d+(?:[-+._A-Za-z0-9]+)?/,
  `$1v${version}`,
)

const changedFiles = [
  [packageJsonFile, packageJsonChanged],
  [packageLockFile, packageLockChanged],
  [dockerComposeFile, dockerComposeChanged],
  [readmeFile, readmeChanged],
].filter(([, changed]) => changed).map(([filePath]) => path.relative(rootDir, filePath))

if (changedFiles.length > 0 && checkOnly) {
  console.error(`Version metadata is not synced to ${version}:`)
  for (const filePath of changedFiles) {
    console.error(`  - ${filePath}`)
  }
  process.exitCode = 1
} else if (changedFiles.length > 0) {
  console.log(`Synced version metadata to ${version}: ${changedFiles.join(', ')}`)
} else {
  console.log(`Version metadata already synced to ${version}`)
}
