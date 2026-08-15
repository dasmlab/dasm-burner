<template>
  <div class="iso-loop">
    <div class="iso-flow" role="list">
      <template v-for="(s, i) in steps" :key="s.id">
        <div class="iso-step" role="listitem">
          <div class="iso-step__n">{{ i + 1 }}</div>
          <div class="iso-step__title">{{ s.title }}</div>
          <div class="iso-step__see">{{ s.see }}</div>
        </div>
        <div v-if="i < steps.length - 1" class="iso-arrow" aria-hidden="true">→</div>
      </template>
    </div>

    <div class="iso-split">
      <div>
        <div class="dasm-stat-label q-mb-xs">Causality</div>
        <div class="iso-chain">
          <template v-for="(c, i) in causality" :key="c">
            <span class="iso-chip">{{ c }}</span>
            <span v-if="i < causality.length - 1" class="iso-chain__arr">→</span>
          </template>
        </div>
        <p class="text-caption text-grey-7 q-mt-sm q-mb-none">
          Recovery (DELETE + finalizers + watch disconnect) is a second mutating load. It belongs in the same loop.
        </p>
      </div>
      <svg class="iso-sketch" viewBox="0 0 320 140" role="img" aria-label="kas RSS stays high after objects are deleted">
        <text x="0" y="14" fill="#1d2b36" font-size="12" font-weight="700">What we measure</text>
        <path :d="rssPath" fill="none" stroke="#c0392b" stroke-width="2.4" />
        <path :d="podPath" fill="none" stroke="#8e6b3a" stroke-width="2" stroke-dasharray="5 4" />
        <text x="0" y="128" fill="#c0392b" font-size="11">kas RSS (stays high)</text>
        <text x="150" y="128" fill="#8e6b3a" font-size="11">objects (gone after delete)</text>
      </svg>
    </div>
  </div>
</template>

<script setup>
defineProps({
  steps: { type: Array, default: () => [] },
  causality: { type: Array, default: () => [] },
})

const rssPath = 'M8,88 C40,70 70,48 110,40 C160,30 200,34 250,38 C280,40 300,42 312,44'
const podPath = 'M8,96 C40,50 70,42 110,42 C150,42 180,70 220,96 C250,112 280,116 312,118'
</script>

<style scoped>
.iso-loop { display: flex; flex-direction: column; gap: 1rem; }
.iso-flow {
  display: flex;
  flex-wrap: wrap;
  align-items: stretch;
  gap: 0.35rem;
}
.iso-step {
  flex: 1 1 9.5rem;
  min-width: 9.5rem;
  max-width: 14rem;
  border: 1px solid var(--dasm-border-strong);
  border-radius: 12px;
  padding: 0.55rem 0.65rem 0.7rem;
  background: #fff;
}
.iso-step__n {
  font-size: 0.68rem;
  letter-spacing: 0.08em;
  color: #1f6f62;
  font-weight: 700;
}
.iso-step__title {
  font-weight: 700;
  color: #1d2b36;
  line-height: 1.25;
  margin: 0.15rem 0 0.25rem;
}
.iso-step__see {
  font-size: 0.78rem;
  color: #607483;
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.iso-arrow {
  align-self: center;
  color: #3f7a6b;
  font-weight: 700;
  padding: 0 0.1rem;
}
.iso-split {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(220px, 0.8fr);
  gap: 1rem;
  align-items: start;
}
@media (max-width: 900px) {
  .iso-split { grid-template-columns: 1fr; }
  .iso-arrow { display: none; }
}
.iso-chain {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.35rem;
}
.iso-chip {
  display: inline-block;
  background: #1f6f62;
  color: #fff;
  font-size: 0.78rem;
  padding: 0.2rem 0.5rem;
  border-radius: 6px;
}
.iso-chain__arr { color: #3f7a6b; font-weight: 700; }
.iso-sketch {
  width: 100%;
  max-width: 320px;
  height: auto;
  background: #f4f7fa;
  border-radius: 10px;
  padding: 0.4rem 0.5rem 0.2rem;
}
</style>
