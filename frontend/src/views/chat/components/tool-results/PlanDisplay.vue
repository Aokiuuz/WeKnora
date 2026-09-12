<template>
  <div class="plan-display">
    <div v-if="data.steps && data.steps.length > 0" class="plan-steps">
      <div v-for="(step, index) in data.steps" :key="step.id || index" class="step-item" :class="`status-${step.status}`">
        <div class="step-checkbox" :class="{ 'checked': step.status === 'completed', 'in-progress': step.status === 'in_progress' }">
          <t-icon :name="step.status === 'completed' ? 'check-rectangle' : 'stop'" size="16px" />
        </div>
        <span class="step-description" :class="{ 'completed': step.status === 'completed' }">
          {{ step.description }}
          <t-icon v-if="step.status === 'in_progress'" name="loading" class="sparkle" />
        </span>
      </div>
    </div>
    
    <div v-else class="no-steps">
      {{ $t('chat.noPlanSteps') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import type { PlanData } from '@/types/tool-results';

interface Props {
  data: PlanData;
}

const props = defineProps<Props>();
</script>

<style lang="less" scoped>
.plan-display {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: transparent;
  padding: 6px 0 6px 12px;
  margin: 0;
  border: none !important;
  box-shadow: none !important;
  outline: none;
}

.plan-steps {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.step-item {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  padding: 1px 0;
  transition: all 0.15s;
  
  &:last-child {
    margin-bottom: 0;
  }
  
  &.status-in_progress {
    .step-description {
      color: var(--td-text-color-primary);
      font-weight: 500;
    }
  }
}

.step-checkbox {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  margin-top: 1px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);

  &.checked {
    color: var(--embed-primary, var(--td-brand-color));
  }

  &.in-progress {
    color: var(--embed-primary, var(--td-brand-color));

    svg rect {
      stroke-width: 2;
    }
  }
}

.step-description {
  flex: 1;
  color: var(--td-text-color-secondary);
  line-height: 1.5;
  font-size: 12px;
  
  &.completed {
    text-decoration: line-through;
    color: var(--td-text-color-placeholder);
  }
  
  .sparkle {
    margin-left: 3px;
    font-size: 11px;
  }
}

.no-steps {
  padding: 12px;
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-style: italic;
  font-size: 12px;
}
</style>

