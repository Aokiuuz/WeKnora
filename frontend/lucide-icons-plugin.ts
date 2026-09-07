import { fileURLToPath } from 'node:url'
import type { Plugin } from 'vite'

/** Restrict private-module adaptation to TDesign's installed ESM distribution. */
export function lucideInternalIcons(): Plugin {
  return {
    name: 'lucide-tdesign-internal-icons',
    enforce: 'pre',
    resolveId(source, importer) {
      if (!importer?.replaceAll('\\', '/').includes('/node_modules/tdesign-vue-next/es/')) return
      if (source.endsWith('/common-components/fake-arrow.mjs')) {
        return fileURLToPath(new URL('./src/components/icons/tdesign-arrow.ts', import.meta.url))
      }
      if (source === './icon/gradient.mjs') {
        return fileURLToPath(new URL('./src/components/icons/tdesign-loading.ts', import.meta.url))
      }
    },
  }
}
