import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync, readdirSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFileSync } from 'node:child_process'
import ts from 'typescript'
import { h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import * as icons from './lucide-compat'
import Arrow from './tdesign-arrow'
import Loading from './tdesign-loading'
import { avatarInitial } from '../../utils/avatarInitial'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
function files(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => entry.isDirectory()
    ? files(resolve(dir, entry.name)) : [resolve(dir, entry.name)])
}
const sourceFiles = files(resolve(root, 'src')).filter(file => /\.(ts|vue)$/.test(file) && !file.includes('.test.'))

test('application and installed TDesign named imports have renderable Lucide exports', async () => {
  const used = new Set<string>()
  for (const file of [...sourceFiles, ...files(resolve(root, 'node_modules/tdesign-vue-next/es')).filter(file => file.endsWith('.mjs'))]) {
    const text = readFileSync(file, 'utf8')
    const scripts = file.endsWith('.vue') ? [...text.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)].map(match => match[1]).join('\n') : text
    const ast = ts.createSourceFile(file, scripts, ts.ScriptTarget.Latest, true)
    for (const node of ast.statements) {
      if (!ts.isImportDeclaration(node) || !ts.isStringLiteral(node.moduleSpecifier) || node.moduleSpecifier.text !== 'tdesign-icons-vue-next') continue
      if (!node.importClause) continue
      const bindings = node.importClause?.namedBindings
      assert.ok(bindings && ts.isNamedImports(bindings), `${file}: use explicit named icons`)
      for (const item of bindings.elements) {
        if (item.isTypeOnly || node.importClause?.isTypeOnly || item.name.text === 'GlobalIconConfig') continue
        used.add(item.propertyName?.text || item.name.text)
      }
    }
  }
  assert.ok(used.size >= 55, `Expected application and library coverage, found ${used.size}`)
  for (const name of used) {
    const component = icons[name as keyof typeof icons]
    assert.ok(component, `Missing ${name}`)
    const html = await renderToString(h(component as typeof icons.Icon, { name: 'search' }))
    assert.match(html, /data-icon-library="lucide"/)
    assert.doesNotMatch(html, /data-icon-fallback/)
    assert.match(html, /<(path|circle|rect|line|polyline)/)
  }
})

test('all static template names are covered, including conditional literal names', () => {
  function returnedNames(expression: ts.Expression): string[] {
    if (ts.isStringLiteral(expression)) return [expression.text]
    if (ts.isConditionalExpression(expression)) return [...returnedNames(expression.whenTrue), ...returnedNames(expression.whenFalse)]
    if (ts.isParenthesizedExpression(expression)) return returnedNames(expression.expression)
    if (ts.isBinaryExpression(expression) && [ts.SyntaxKind.BarBarToken, ts.SyntaxKind.QuestionQuestionToken].includes(expression.operatorToken.kind)) {
      return [...returnedNames(expression.left), ...returnedNames(expression.right)]
    }
    return []
  }
  const missing: string[] = []
  for (const file of sourceFiles.filter(file => file.endsWith('.vue'))) {
    const source = readFileSync(file, 'utf8')
    for (const tag of source.matchAll(/<(?:t-icon|TIcon)\b[^>]*>/g)) {
      const staticName = tag[0].match(/\sname="([^"]+)"/)
      const dynamicName = tag[0].match(/:name="([^"]+)"/)
      const statement = ts.createSourceFile('icon.ts', dynamicName?.[1] || '', ts.ScriptTarget.Latest, true).statements[0]
      const names = staticName ? [staticName[1]] : statement && ts.isExpressionStatement(statement) ? returnedNames(statement.expression) : []
      for (const name of names) if (!Object.hasOwn(icons.iconComponents, name)) missing.push(`${file}: ${name}`)
    }
  }
  assert.deepEqual(missing, [])
})

test('unknown and prototype names have visible fallback and one diagnostic per name', async () => {
  const warnings: unknown[][] = []
  const original = console.warn
  console.warn = (...args) => warnings.push(args)
  try {
    for (const name of ['fixture-unknown', '__proto__', 'constructor']) {
      for (let pass = 0; pass < 2; pass++) {
        const html = await renderToString(h(icons.Icon, { name }))
        assert.match(html, /data-icon-fallback="true"/)
        assert.match(html, /<path/)
      }
    }
    assert.equal(warnings.length, 3)
  } finally { console.warn = original }
})

test('icon sizing, rotation, classes, explicit accessibility and library state are retained', async () => {
  const html = await renderToString(h(icons.Icon, { name: 'search', size: 28, rotate: 90, class: 'control', 'aria-label': 'Search', role: 'img', style: { color: 'red' } }))
  assert.match(html, /width="28"/)
  assert.match(html, /control/)
  assert.match(html, /rotate\(90deg\)/)
  assert.match(html, /color:red/)
  assert.match(html, /aria-label="Search"/)
  assert.doesNotMatch(html, /aria-hidden="true"/)
  assert.match(await renderToString(h(icons.Icon, { name: 'search' })), /aria-hidden="true"/)
  const arrow = await renderToString(h(Arrow, { isActive: true, overlayClassName: 'fixture-arrow', overlayStyle: 'color:red' }))
  assert.match(arrow, /app-select-arrow--active/)
  assert.match(arrow, /fixture-arrow/)
  assert.match(arrow, /color:red/)
  assert.match(await renderToString(h(Loading)), /app-icon-spin/)
})

test('avatar initials use letters or numbers without changing supplied values', () => {
  for (const [value, expected] of [['\u{1f680} 课题三', '课'], ['  alice', 'A'], ['\u{1f1e8}\u{1f1f3}', '?'], ['3D', '3']]) {
    assert.equal(avatarInitial(value), expected)
  }
  const source = readFileSync(resolve(root, 'src/components/SpaceAvatar.vue'), 'utf8')
  assert.match(source, /startsWith\('icon:'\)/)
  assert.doesNotMatch(source, /v-html|defineEmits|props\.avatar\s*=/)
})

test('fixed SVG assets and trusted HTML glyphs match the installed Lucide package', () => {
  const result = execFileSync(process.execPath, ['scripts/generate-lucide-assets.mjs', '--check'], { cwd: root, encoding: 'utf8' })
  assert.match(result, /Verified 37/)
})

test('application UI source has no emoji literals or remote icon-font injection', () => {
  const violations: string[] = []
  for (const file of sourceFiles) {
    const raw = readFileSync(file, 'utf8')
    // Comments and an internal graph-edge identity delimiter do not render UI text.
    const source = raw.replace(/<!--[\s\S]*?-->|\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, '')
      .replaceAll("'↔'", "''")
    if (/[\p{Extended_Pictographic}\p{Regional_Indicator}]/u.test(source)) violations.push(file)
  }
  assert.deepEqual(violations, [])
  for (const entry of ['index.html', 'embed.html']) {
    assert.doesNotMatch(readFileSync(resolve(root, entry), 'utf8'), /tdesign-icons\/.*fonts|iconfont/i)
  }
})
