import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const rootDir = path.resolve(__dirname, '..')

const versionFile = path.join(rootDir, 'config', 'version')
const packageJsonFile = path.join(rootDir, 'temp_frontend', 'package.json')
const packageLockFile = path.join(rootDir, 'temp_frontend', 'package-lock.json')

const version = fs.readFileSync(versionFile, 'utf8').trim()

if (!/^\d+\.\d+\.\d+(?:[-+._A-Za-z0-9]+)?$/.test(version)) {
  throw new Error(`Invalid version in ${versionFile}: ${version}`)
}

const updateJsonFile = (filePath, updater) => {
  const original = fs.readFileSync(filePath, 'utf8')
  const parsed = JSON.parse(original)
  const changed = updater(parsed)

  if (!changed) {
    return false
  }

  fs.writeFileSync(filePath, `${JSON.stringify(parsed, null, 2)}\n`, 'utf8')
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

if (packageJsonChanged || packageLockChanged) {
  console.log(`Synced frontend version metadata to ${version}`)
} else {
  console.log(`Version metadata already synced to ${version}`)
}
