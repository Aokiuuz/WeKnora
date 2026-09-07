import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { AtSign, BookOpen, Bot, Brain, ClipboardCheck, Copy, Download, FilePlus,
  FileText, Globe, LogOut, Maximize, MessageSquarePlus, Minimize2, Plug,
  RotateCcw, Rss, Search, Send, Settings, Trash2, User, Users, X, ZoomIn, ZoomOut } from '@lucide/vue'

// Functional image assets retain their existing URL, dimensions and state colors.
// Brand marks, document illustrations and user-supplied graphics are separate assets.
const groups = [
  [Bot, ['agent', 'agent-green', 'agent-active']], [AtSign, ['at-icon']],
  [Minimize2, ['context-compaction']], [Rss, ['datasource-rss']],
  [Trash2, ['delete']], [Download, ['download']],
  [ClipboardCheck, ['evaluation', 'evaluation-green']],
  [FilePlus, ['file-add', 'file-add-green', 'file-add-icon']],
  [Brain, ['Frame3718']], [Plug, ['integration', 'integration-green']],
  [LogOut, ['logout']], [Users, ['organization', 'organization-green', 'organization-grey']],
  [MessageSquarePlus, ['prefixIcon', 'prefixIcon-green', 'prefixIcon-grey']],
  [Search, ['search', 'search-green']], [Send, ['sending-aircraft']],
  [Settings, ['setting', 'setting-green']], [User, ['user', 'user-green']],
  [Globe, ['websearch-globe', 'websearch-globe-green']],
  [BookOpen, ['zhishiku', 'zhishiku-green', 'zhishiku-thin']], [FileText, ['ziliao']],
]
const root = new URL('../', import.meta.url)
const check = process.argv.includes('--check')
let count = 0
function output(relative, value) {
  const path = fileURLToPath(new URL(relative, root))
  if (check) {
    if (readFileSync(path, 'utf8').replaceAll('\r\n', '\n') !== value) throw new Error(`Lucide asset drift: ${relative}`)
  } else writeFileSync(path, value)
  count++
}
for (const [component, names] of groups) {
  for (const name of names) {
    const relative = `src/assets/img/${name}.svg`
    const previous = readFileSync(new URL(relative, root), 'utf8')
    const width = previous.match(/\bwidth="([^"]+)"/)?.[1] || '20'
    const height = previous.match(/\bheight="([^"]+)"/)?.[1] || width
    const color = name === 'sending-aircraft' ? '#ffffff'
      : /green|active/.test(name) ? '#07c05f' : /grey/.test(name) ? '#9ca3af' : '#4b5563'
    const svg = await renderToString(h(component, { width, height, color, strokeWidth: 1.75, 'aria-hidden': 'true', 'data-icon-library': 'lucide' }))
    output(relative, `<!-- Generated from @lucide/vue (ISC). Run node scripts/generate-lucide-assets.mjs. -->\n${svg}\n`)
  }
}
const markup = {}
for (const [name, component, size, className] of [
  ['copy', Copy, 16, 'chat-code-block__copy-icon'], ['expand', Maximize, 16, 'chat-mermaid-block__expand-icon'],
  ['zoomIn', ZoomIn, 18], ['zoomOut', ZoomOut, 18], ['reset', RotateCcw, 18],
  ['download', Download, 18], ['close', X, 18], ['file', FileText, 24, 'artifact-ref-card__glyph'],
]) {
  markup[name] = await renderToString(h(component, { size, class: className, strokeWidth: 1.75, 'aria-hidden': 'true', 'data-icon-library': 'lucide' }))
}
output('src/components/icons/lucide-markup.ts', '// Generated from @lucide/vue (ISC). Run node scripts/generate-lucide-assets.mjs.\n// Only fixed, trusted glyphs are serialized; caller content never enters this map.\nexport const lucideMarkup = ' + JSON.stringify(markup, null, 2) + ' as const\n')
console.log(`${check ? 'Verified' : 'Generated'} ${count} Lucide asset files`)
