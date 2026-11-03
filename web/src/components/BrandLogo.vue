<template>
  <div class="brand-logo" :class="{ collapsed }">
    <img class="logo-img" :src="logo" :alt="$t('松鼠下载器')" />
    <div class="logo-text" :class="{ hidden: collapsed }" :aria-hidden="collapsed">
      <span class="logo-name">{{ $t('松鼠下载器') }}</span>
      <span class="logo-sub">Squirrel Downloader</span>
    </div>
  </div>
</template>

<script setup>
import logo from '../assets/logo.png'

defineProps({
  collapsed: {
    type: Boolean,
    default: false
  }
})
</script>

<style scoped>
.brand-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  height: 60px;
  overflow: hidden;
  transition: padding 0.3s cubic-bezier(0.2, 0, 0, 1), gap 0.3s cubic-bezier(0.2, 0, 0, 1);
}

.brand-logo.collapsed {
  justify-content: center;
  padding: 0;
  gap: 0;
}

.logo-img {
  flex-shrink: 0;
  width: 34px;
  height: 34px;
  border-radius: var(--radius-sm);
  display: block;
  object-fit: contain;
}

.logo-text {
  display: flex;
  flex-direction: column;
  line-height: 1.25;
  white-space: nowrap;
  overflow: hidden;
  max-width: 200px;
  transition: max-width 0.3s cubic-bezier(0.2, 0, 0, 1), opacity 0.2s ease;
}

/* 收起时不直接 v-if 移除，否则文字会在第一帧凭空消失。
 * 用 max-width 收起而不是 absolute：文字留在 flex 流里，会随侧栏一起变窄、
 * 被裁切并淡出，logo 也会平滑地被挤到中线，不会出现文字压在图标上的重影。 */
.logo-text.hidden {
  max-width: 0;
  opacity: 0;
  pointer-events: none;
}

.logo-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-1);
  letter-spacing: 0.2px;
}

.logo-sub {
  font-size: 11px;
  color: var(--text-3);
}
</style>
