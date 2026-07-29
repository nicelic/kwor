import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const rootDir = path.resolve(__dirname, '..')
const distDir = path.join(rootDir, 'temp_frontend', 'dist')
const embeddedDir = path.join(rootDir, 'web', 'html')

if (!fs.existsSync(path.join(distDir, 'index.html'))) {
  throw new Error(`Frontend build output is missing: ${distDir}`)
}

// web/html is the directory compiled into the Go binary. Keep this copy in
// the frontend build itself so a successful frontend build cannot leave stale
// embedded assets behind.
fs.rmSync(embeddedDir, { recursive: true, force: true })
fs.cpSync(distDir, embeddedDir, { recursive: true })

if (!fs.existsSync(path.join(embeddedDir, 'index.html'))) {
  throw new Error(`Embedded frontend sync failed: ${embeddedDir}`)
}

console.log(`Synced frontend build to ${embeddedDir}`)
