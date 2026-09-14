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

// Prune legacy redundant font files (.ttf, .eot, .woff) to save Go binary .rodata RAM
function pruneLegacyFonts(dir) {
  if (!fs.existsSync(dir)) return
  const files = fs.readdirSync(dir, { withFileTypes: true })
  for (const file of files) {
    const fullPath = path.join(dir, file.name)
    if (file.isDirectory()) {
      pruneLegacyFonts(fullPath)
    } else if (file.isFile()) {
      const ext = path.extname(file.name).toLowerCase()
      if (ext === '.ttf' || ext === '.eot' || ext === '.woff') {
        fs.unlinkSync(fullPath)
      }
    }
  }
}
pruneLegacyFonts(path.join(embeddedDir, 'assets'))

if (!fs.existsSync(path.join(embeddedDir, 'index.html'))) {
  throw new Error(`Embedded frontend sync failed: ${embeddedDir}`)
}

console.log(`Synced frontend build to ${embeddedDir}`)
