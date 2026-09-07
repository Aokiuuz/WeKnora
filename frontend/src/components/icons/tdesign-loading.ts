import { defineComponent, h } from 'vue'
import { Icon } from './lucide-compat'

/** Loading owns delay and overlay behavior; this supplies only its glyph. */
export default defineComponent({
  name: 'LucideLoadingGlyph',
  setup: () => () => h(Icon, { name: 'loading', size: '1em' }),
})
