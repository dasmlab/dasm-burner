<template>
  <div class="src-hub" role="list">
    <button
      v-for="p in pieces"
      :key="p.id"
      type="button"
      class="src-piece"
      :class="{ 'is-sel': selectedId === p.id }"
      role="listitem"
      @click="$emit('select', p)"
    >
      <div class="src-piece__name">{{ p.name }}</div>
      <div class="src-piece__sha"><code>{{ short(p.payloadSha) }}</code></div>
      <div class="src-piece__role">{{ p.role }}</div>
    </button>
  </div>
</template>

<script setup>
defineProps({
  cluster: { type: String, default: '' },
  openshift: { type: String, default: '' },
  pieces: { type: Array, default: () => [] },
  selectedId: { type: String, default: '' },
})
defineEmits(['select'])

function short(sha) {
  return sha ? sha.slice(0, 8) : '—'
}
</script>

<style scoped>
.src-hub {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.65rem;
}
@media (max-width: 640px) {
  .src-hub { grid-template-columns: 1fr; }
}
.src-piece {
  text-align: left;
  border: 1px solid var(--dasm-border-soft);
  border-radius: 12px;
  padding: 0.7rem 0.8rem;
  background: #fff;
  cursor: pointer;
  min-height: 0;
}
.src-piece.is-sel {
  border-color: var(--dasm-border-strong);
  background: rgba(63, 122, 107, 0.08);
}
.src-piece__name {
  font-weight: 700;
  color: #1d2b36;
}
.src-piece__sha {
  font-size: 0.78rem;
  color: #607483;
  margin: 0.15rem 0 0.35rem;
}
.src-piece__role {
  font-size: 0.8rem;
  color: #445566;
  line-height: 1.35;
}
</style>
