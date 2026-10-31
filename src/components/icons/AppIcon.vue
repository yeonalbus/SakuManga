/**
 * AppIcon —— 全站统一图标入口（Phosphor）
 *
 * 用法：
 *   <AppIcon name="settings" />                     // 默认 16px 空心
 *   <AppIcon name="heart" :weight="isFav ? 'fill' : 'regular'" :size="16" />
 *   <AppIcon name="loading" class="spin" />         // 加载旋转（.spin 见 App.vue 全局样式）
 *
 * name 来自 iconMap.ts 的语义 key，拼错会 type-check 报错。
 * 颜色默认继承 currentColor，status 色由调用方样式控制。
 */
<script setup lang="ts">
import type { Component } from 'vue'
import { computed } from 'vue'
import { ICONS } from './iconMap'
import type { IconName } from './iconMap'

const props = withDefaults(
  defineProps<{
    name: IconName
    /** 图标尺寸（px），默认 16 */
    size?: number
    /** regular=空心（默认）；fill=实心（状态位激活时用） */
    weight?: 'regular' | 'fill' | 'bold'
    /** 颜色，缺省继承 currentColor */
    color?: string
  }>(),
  { size: 16, weight: 'regular' },
)

const Comp = computed<Component>(() => ICONS[props.name])
</script>

<template>
  <component
    :is="Comp"
    :size="size"
    :weight="weight"
    :color="color"
    class="app-icon"
    aria-hidden="true"
  />
</template>
