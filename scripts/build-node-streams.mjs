import { build } from 'esbuild'

const entry = 'streams/internal/streamx/facade.js'
const outfile = 'streams/internal/streamx/bundle.js'

await build({
  entryPoints: [entry],
  outfile,
  bundle: true,
  format: 'cjs',
  target: 'es2015',
  platform: 'browser',
  globalName: 'GojaNodeStream',
  external: ['events', 'goja:stream/web'],
  alias: { 'events-universal': 'events' },
  define: {
    'process.env.NODE_ENV': '"production"'
  }
})

const { readFileSync } = await import('node:fs')
const { createHash } = await import('node:crypto')
const source = readFileSync(outfile, 'utf8')
console.log(`built ${outfile} (${source.length} bytes) sha256=${createHash('sha256').update(source).digest('hex')}`)
