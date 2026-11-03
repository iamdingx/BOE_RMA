<template>
  <div class="pager">
    <el-pagination
      background layout="total, sizes, prev, pager, next"
      :total="total" :current-page="page" :page-size="size"
      :page-sizes="[10, 20, 50, 200]"
      @current-change="(p) => emit('change', p, size)"
      @size-change="(s) => emit('change', 1, s)"
    />
  </div>
</template>

<script setup>
defineProps({ total: { type: Number, default: 0 }, page: { type: Number, default: 1 }, size: { type: Number, default: 20 } })
const emit = defineEmits(['change'])
</script>

<style scoped>
.pager { display: flex; justify-content: flex-end; margin-top: 12px; }
</style>
