import { defineComponent, h, type PropType, type StyleValue } from 'vue'
import { Icon } from './lucide-compat'

/** Preserve the select arrow's active state and overlay attributes. */
export default defineComponent({
  name: 'LucideSelectArrow',
  props: {
    isActive: Boolean,
    overlayClassName: [String, Object, Array],
    overlayStyle: [Object, String] as PropType<StyleValue>,
  },
  setup: (props) => () => h(Icon, {
    name: 'chevron-down', size: '1em',
    class: ['app-select-arrow', { 'app-select-arrow--active': props.isActive }, props.overlayClassName],
    style: props.overlayStyle,
  }),
})
