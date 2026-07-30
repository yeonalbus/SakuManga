<template>
  <div class="mouse-wheel-settings">
    <div class="setting-item">
      <div class="item-info">
        <div class="item-title">鼠标滚轮速度</div>
      </div>
      <div class="input-inline">
        <input type="number" step="0.1" v-model="wheelSpeed" class="setting-input" />
        <span class="check-mark">✓</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const wheelSpeed = ref(5.0)
</script>

<style scoped>
.mouse-wheel-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.setting-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background-color: #1a1a1e;
  border-radius: 8px;
  border: 1px solid #26262a;
}

.item-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.item-title {
  font-size: 15px;
  font-weight: 500;
  color: #ffffff;
}

.input-inline {
  display: flex;
  align-items: center;
  gap: 6px;
}

.setting-input {
  background: transparent;
  border: none;
  border-bottom: 1px solid #44444a;
  color: #ffffff;
  font-size: 14px;
  width: 45px;
  text-align: center;
  outline: none;
}

.setting-input:focus {
  border-bottom-color: #ff7588;
}

.check-mark {
  color: #a0a0a5;
  font-size: 14px;
  margin-left: 4px;
}
</style>
