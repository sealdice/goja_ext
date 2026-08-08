import { build } from 'esbuild'
import { createHash } from 'node:crypto'
import { copyFileSync, readFileSync } from 'node:fs'

const entry = 'fetch/internal/bare/facade.js'
const outfile = 'fetch/internal/bare/bundle.js'

await build({
  entryPoints: [entry],
  outfile,
  bundle: true,
  format: 'cjs',
  target: 'es2015',
  platform: 'browser',
  external: ['goja:stream/web', 'goja:url', 'goja:buffer'],
  alias: {
    'bare-stream/web': 'goja:stream/web',
    'bare-url': 'goja:url',
    'bare-buffer': 'goja:buffer'
  },
  define: {
    'process.env.NODE_ENV': '"production"'
  }
})

copyFileSync('node_modules/bare-fetch/LICENSE', 'fetch/internal/bare/LICENSE.bare-fetch')
copyFileSync('node_modules/bare-form-data/LICENSE', 'fetch/internal/bare/LICENSE.bare-form-data')
copyFileSync('node_modules/bare-mime/LICENSE', 'fetch/internal/bare/LICENSE.bare-mime')

const source = readFileSync(outfile, 'utf8')
const digest = createHash('sha256').update(source).digest('hex')
console.log(`built ${outfile} (${source.length} bytes) sha256=${digest}`)
