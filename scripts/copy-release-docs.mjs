import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const rootDir = path.resolve(__dirname, '..')

const releaseDocs = [
  'User Manual.md',
  '使用手册.md',
]

const destinationArg = process.argv[2]
if (!destinationArg) {
  throw new Error('Usage: node scripts/copy-release-docs.mjs <destination-directory>')
}

const destinationDir = path.resolve(rootDir, destinationArg)
fs.mkdirSync(destinationDir, { recursive: true })

for (const fileName of releaseDocs) {
  const sourcePath = path.join(rootDir, fileName)
  const targetPath = path.join(destinationDir, fileName)

  if (!fs.existsSync(sourcePath)) {
    throw new Error(`Release document not found: ${sourcePath}`)
  }

  fs.copyFileSync(sourcePath, targetPath)
}

console.log(`Copied release docs to ${destinationDir}`)
