<template>
  <div class="brand-logo" :class="{ collapsed }">
    <img class="logo-img" :src="logo" alt="松鼠下载器" />
    <div v-if="!collapsed" class="logo-text">
      <span class="logo-name">松鼠下载器</span>
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
}

.brand-logo.collapsed {
  justify-content: center;
  padding: 0;
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
