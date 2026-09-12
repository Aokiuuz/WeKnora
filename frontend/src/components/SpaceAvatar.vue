<template>
  <div class="space-avatar" :style="avatarStyle"
    :class="{ 'space-avatar-small': size === 'small', 'space-avatar-large': size === 'large' }">
    <Icon v-if="iconName" :name="iconName" :size="size === 'large' ? 26 : size === 'small' ? 14 : 20" class="space-avatar-icon" />
    <template v-else>
      <Icon name="relation" class="space-avatar-decoration" aria-hidden="true" />
      <span class="space-avatar-letter" :style="letterStyle">{{ letter }}</span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@/components/icons/lucide-compat'
import { avatarInitial } from '@/utils/avatarInitial'

const props = withDefaults(defineProps<{
  name: string
  /** Saved avatar values are preserved; icon: names select Lucide presentation. */
  avatar?: string
  size?: 'small' | 'medium' | 'large'
}>(), { size: 'medium', avatar: '' })
const iconName = computed(() => props.avatar.startsWith('icon:') ? props.avatar.slice(5) : '')
const letter = computed(() => avatarInitial(props.name))
const gradients = [
  { from: '#07c05f', to: '#059669' },  // 主绿
  { from: '#11998e', to: '#38ef7d' },  // 深绿渐变
  { from: '#43e97b', to: '#38f9d7' },  // 绿青
  { from: '#02aab0', to: '#00cdac' },  // 青绿
  { from: '#36d1dc', to: '#5b86e5' }, // 青蓝
  { from: '#4facfe', to: '#00f2fe' },  // 蓝青
  { from: '#667eea', to: '#764ba2' },  // 紫蓝
  { from: '#4776e6', to: '#8e54e9' },  // 蓝紫
  { from: '#56ab2f', to: '#a8e063' },  // 草绿
  { from: '#00b09b', to: '#96c93d' },  // 青绿
  { from: '#5ee7df', to: '#b490ca' },  // 青紫
  { from: '#614385', to: '#516395' },  // 深紫蓝
];
const gradient = computed(() => {
  let hash = 0
  for (const character of props.name) hash = ((hash << 5) - hash + character.charCodeAt(0)) | 0
  return gradients[Math.abs(hash) % gradients.length]!
})
const avatarStyle = computed(() => ({ background: `linear-gradient(135deg, ${gradient.value.from} 0%, ${gradient.value.to} 100%)` }))
const letterStyle = computed(() => ({ textShadow: `0 1px 2px ${gradient.value.to}80, 0 0 8px ${gradient.value.from}30` }))
</script>

<style scoped lang="less">
.space-avatar {
  position: relative; display: flex; align-items: center; justify-content: center;
  width: 32px; height: 32px; border-radius: 8px; flex-shrink: 0;
  box-shadow: var(--td-shadow-2); overflow: hidden;
  &.space-avatar-small { width: 22px; height: 22px; border-radius: 5px; box-shadow: none;
    .space-avatar-letter { font-size: 11px; }
    .space-avatar-decoration { display: none; }
  }
  &.space-avatar-large { width: 48px; height: 48px; border-radius: 12px;
    .space-avatar-letter { font-size: 20px; }
  }
}
.space-avatar-decoration { position: absolute; right: 0; bottom: 0; width: 55%; height: 55%; opacity: .35; color: white; pointer-events: none; }
.space-avatar-letter, .space-avatar-icon { position: relative; z-index: 1; color: var(--td-text-color-anti); font-size: 14px; font-weight: 600; font-family: var(--app-font-family); }
</style>
