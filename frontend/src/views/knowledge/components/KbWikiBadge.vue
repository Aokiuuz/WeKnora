<template>
  <t-tooltip :content="t('knowledgeList.features.wiki')" placement="top">
    <span class="kb-wiki-badge" role="img" :aria-label="t('knowledgeList.features.wiki')">
      <t-icon name="book" size="1em" aria-hidden="true" />
    </span>
  </t-tooltip>
</template>

<script setup lang="ts">
// Icon: Lucide "book-open-text" (ISC) — https://lucide.dev/icons/book-open-text
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<style scoped lang="less">
.kb-wiki-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: var(--td-text-color-secondary);

  svg {
    width: 15px;
    height: 15px;
    display: block;
  }
}
</style>
