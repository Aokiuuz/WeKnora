<template>
  <div 
    class="agent-avatar" 
    :style="avatarStyle"
    :class="{ 'agent-avatar-small': size === 'small', 'agent-avatar-large': size === 'large' }"
  >
    <Sparkles class="agent-sparkles" aria-hidden="true" />
    <span class="agent-avatar-letter" :style="letterStyle">{{ letter }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { Sparkles } from '@lucide/vue';
import { avatarInitial } from '@/utils/avatarInitial';

const props = withDefaults(defineProps<{
  name: string;
  size?: 'small' | 'medium' | 'large';
}>(), {
  size: 'medium'
});

// 预定义的渐变色方案 - 现代、柔和、专业
const gradients = [
  { from: '#667eea', to: '#764ba2' },  // 紫蓝渐变
  { from: '#4facfe', to: '#00f2fe' },  // 蓝青渐变
  { from: '#43e97b', to: '#38f9d7' },  // 绿青渐变
  { from: '#11998e', to: '#38ef7d' },  // 深绿渐变
  { from: '#5ee7df', to: '#b490ca' },  // 青紫渐变
  { from: '#48c6ef', to: '#6f86d6' },  // 蓝紫渐变
  { from: '#a8edea', to: '#fed6e3' },  // 青粉渐变（柔和）
  { from: '#667db6', to: '#0082c8' },  // 蓝色渐变
  { from: '#36d1dc', to: '#5b86e5' },  // 青蓝渐变
  { from: '#56ab2f', to: '#a8e063' },  // 草绿渐变
  { from: '#614385', to: '#516395' },  // 深紫蓝渐变
  { from: '#02aab0', to: '#00cdac' },  // 青绿渐变
  { from: '#6a82fb', to: '#fc5c7d' },  // 蓝粉渐变（柔和）
  { from: '#834d9b', to: '#d04ed6' },  // 紫色渐变
  { from: '#4776e6', to: '#8e54e9' },  // 蓝紫渐变
  { from: '#00b09b', to: '#96c93d' },  // 青绿渐变
];

// 根据名称生成一个稳定的哈希值
const hashCode = (str: string): number => {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash;
  }
  return Math.abs(hash);
};

// 获取首字母（支持中文）
const letter = computed(() => avatarInitial(props.name));

// 根据名称选择渐变色
const gradient = computed(() => {
  const hash = hashCode(props.name || '');
  return gradients[hash % gradients.length];
});

// 生成样式
const avatarStyle = computed(() => {
  const g = gradient.value;
  return {
    background: `linear-gradient(135deg, ${g.from} 0%, ${g.to} 100%)`
  };
});

// 字母样式 - 白色 + 背景色阴影增加层次感
const letterStyle = computed(() => {
  const g = gradient.value;
  return {
    textShadow: `0 1px 2px ${g.to}80, 0 0 8px ${g.from}30`
  };
});
</script>

<style scoped lang="less">
.agent-avatar {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  flex-shrink: 0;
  box-shadow: var(--td-shadow-2);
  overflow: hidden;
  
  &.agent-avatar-small {
    width: 22px;
    height: 22px;
    border-radius: 5px;
    box-shadow: none;
    
    .agent-avatar-letter {
      font-size: 11px;
    }
    
    .agent-sparkles {
      display: none;
    }
  }
  
  &.agent-avatar-large {
    width: 48px;
    height: 48px;
    border-radius: 12px;
    
    .agent-avatar-letter {
      font-size: 20px;
    }
    
    .agent-sparkles {
      opacity: 0.9;
    }
  }
}

.agent-sparkles {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  opacity: 0.85;
}

.agent-avatar-letter {
  position: relative;
  z-index: 1;
  color: var(--td-text-color-anti);
  font-size: 14px;
  font-weight: 600;
  font-family: var(--app-font-family);
}
</style>
