import { build } from 'esbuild'
import { createHash } from 'node:crypto'
import { copyFileSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

const packageName = '@microsoft/fetch-event-source'
const version = '2.0.1'
const directory = 'fetch/internal/fetch_event_source'
const outfile = `${directory}/bundle.js`
const patchedParser = resolve(directory, 'parse.js')

await build({
  entryPoints: [`${directory}/entry.js`],
  outfile,
  bundle: true,
  format: 'cjs',
  target: 'es2015',
  platform: 'browser',
  inject: [`${directory}/headless-shim.js`],
  plugins: [{
    name: 'fetch-event-source-parser-fix',
    setup(build) {
      build.onResolve({ filter: /^\.\/parse$/ }, (args) => {
        const importer = args.importer.replaceAll('\\', '/')
        if (importer.includes('/@microsoft/fetch-event-source/')) {
          return { path: patchedParser }
        }
      })
    }
  }],
  define: {
    'process.env.NODE_ENV': '"production"'
  }
})

copyFileSync(
  `node_modules/${packageName}/LICENSE`,
  `${directory}/LICENSE.fetch-event-source`
)

const digest = createHash('sha256').update(readFileSync(outfile)).digest('hex')
writeFileSync(`${directory}/README.md`, `# Embedded fetch-event-source\n\n` +
  `- Upstream: \`${packageName}@${version}\`\n` +
  `- Bundle SHA-256: \`${digest}\`\n` +
  `- Rebuild: \`npm run build:fetch-event-source\`\n\n` +
  `The esbuild inject shim supplies private headless \`window\` and \`document\` ` +
  `objects plus the runtime's canonical \`AbortController\` and \`TextDecoder\`. ` +
  `The pinned parser is patched to ignore comment-only blocks while preserving ` +
  `explicit empty \`data:\` events.\n`)
