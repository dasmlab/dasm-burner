<template>
  <div class="topo-wrap" :class="{ compact }">
    <svg
      class="topo-svg"
      :viewBox="`0 0 ${vbW} ${vbH}`"
      :style="{ height: svgH + 'px' }"
      preserveAspectRatio="xMidYMid meet"
      @dragover.prevent
      @drop.prevent="onDrop"
    >
      <defs>
        <pattern id="grid" width="28" height="28" patternUnits="userSpaceOnUse">
          <path d="M 28 0 L 0 0 0 28" fill="none" stroke="rgba(47,143,125,0.08)" stroke-width="1" />
        </pattern>
        <marker id="arrow" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L6,3 L0,6 Z" fill="#70835a" />
        </marker>
        <filter id="softShadow" x="-20%" y="-20%" width="140%" height="140%">
          <feDropShadow dx="0" dy="3" stdDeviation="4" flood-color="#12202c" flood-opacity="0.14" />
        </filter>
      </defs>
      <rect width="100%" height="100%" fill="url(#grid)" />

      <g v-if="model.namespaces > 0" filter="url(#softShadow)">
        <rect
          :x="ns.x"
          :y="ns.y"
          :width="ns.w"
          :height="ns.h"
          rx="16"
          class="box-ns"
          :class="{ active: selected === 'ns' }"
          @click.stop="select('ns')"
        />
        <text :x="ns.x + 18" :y="ns.y + 28" class="box-title">Namespace</text>
        <text :x="ns.x + ns.w - 18" :y="ns.y + 28" text-anchor="end" class="box-count">× {{ model.namespaces }}</text>
        <text :x="ns.x + 18" :y="ns.y + 48" class="box-sub">{{ nsSubtitle }}</text>
      </g>

      <path v-for="e in edges" :key="e.id" :d="e.d" class="link" fill="none" marker-end="url(#arrow)" />

      <g v-for="n in inner" :key="n.id" filter="url(#softShadow)" @click.stop="select(n.id)">
        <rect
          :x="n.x"
          :y="n.y"
          :width="n.w"
          :height="n.h"
          rx="12"
          :class="['box-inner', n.cls, { active: selected === n.id, dim: n.dim }]"
        />
        <text :x="n.x + 14" :y="n.y + 26" class="inner-title">{{ n.label }}</text>
        <text :x="n.x + n.w - 14" :y="n.y + 26" text-anchor="end" class="inner-count">× {{ n.count }}</text>
        <text :x="n.x + 14" :y="n.y + 46" class="inner-sub">{{ n.sub }}</text>
      </g>

      <text v-if="model.namespaces < 1" x="40" y="80" class="box-sub">Drop a Namespace from the palette to start.</text>
    </svg>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  model: { type: Object, required: true },
  selected: { type: String, default: '' },
  compact: { type: Boolean, default: false },
})
const emit = defineEmits(['select', 'drop'])

const vbW = computed(() => (props.compact ? 640 : 920))
const isPressure = computed(() => props.model.kind === 'OpenShiftObjectPressure')
const pressureCols = computed(() => (props.compact ? 2 : 3))

const inner = computed(() => {
  if (props.model.namespaces < 1) return []
  if (isPressure.value) {
    const objs = (props.model.objects || []).filter((o) => o.enabled)
    const cols = pressureCols.value
    const cellW = props.compact ? 280 : 270
    const padX = props.compact ? 36 : 72
    return objs.map((o, i) => {
      const col = i % cols
      const row = Math.floor(i / cols)
      const gvk = o.apiVersion || ''
      const extra = o.clusterScoped ? ' · cluster' : (o.custom ? ' · custom CRD' : '')
      return {
        id: o.id,
        label: o.kind || o.id,
        count: o.replicasPerNamespace || 1,
        sub: (gvk || (o.custom ? 'custom GVK' : '')) + extra,
        cls: o.custom ? 'is-custom' : (o.clusterScoped ? 'is-authz' : 'is-pressure'),
        dim: false,
        x: padX + col * cellW,
        y: 88 + row * 86,
        w: props.compact ? 260 : 250,
        h: 68,
      }
    })
  }
  return [
    {
      id: 'route',
      label: 'Route',
      count: props.model.routesPerNamespace,
      sub: 'objectTemplate replicas · {{.Replica}}',
      cls: 'is-route',
      x: 72,
      y: 110,
      w: 230,
      h: 72,
    },
    {
      id: 'service',
      label: 'Service',
      count: props.model.servicesPerNamespace,
      sub: 'objectTemplate replicas · {{.Replica}}',
      cls: 'is-svc',
      x: 345,
      y: 110,
      w: 230,
      h: 72,
    },
    {
      id: 'pod',
      label: 'Pod',
      count: props.model.replicasPerService,
      sub: 'Deployment spec.replicas per service',
      cls: 'is-pod',
      x: 618,
      y: 110,
      w: 230,
      h: 72,
    },
  ]
})

const pressureRows = computed(() =>
  Math.max(1, Math.ceil(((props.model.objects || []).filter((o) => o.enabled).length) / pressureCols.value)),
)
const vbH = computed(() => (isPressure.value ? Math.max(props.compact ? 320 : 420, 116 + pressureRows.value * 86) : 420))
const svgH = computed(() => Math.min(props.compact ? 640 : 760, Math.max(props.compact ? 280 : 420, vbH.value * (props.compact ? 0.72 : 0.58))))
const ns = computed(() => ({ x: 20, y: 20, w: vbW.value - 40, h: vbH.value - 40 }))
const nsSubtitle = computed(() =>
  isPressure.value
    ? 'kube-burner jobIterations · object-pressure init'
    : 'kube-burner jobIterations · namespacedIterations',
)

const edges = computed(() => {
  if (isPressure.value) return []
  const nodes = inner.value
  if (nodes.length < 3) return []
  const a = nodes[0]
  const b = nodes[1]
  const c = nodes[2]
  const mid = (n) => ({ x: n.x + n.w / 2, y: n.y + n.h })
  const ra = mid(a)
  const rb = mid(b)
  const rc = mid(c)
  return [
    {
      id: 'rs',
      d: `M ${ra.x} ${ra.y + 8} C ${ra.x} ${ra.y + 70}, ${rb.x} ${rb.y + 70}, ${rb.x} ${rb.y + 8}`,
    },
    {
      id: 'sp',
      d: `M ${rb.x} ${rb.y + 8} C ${rb.x} ${rb.y + 70}, ${rc.x} ${rc.y + 70}, ${rc.x} ${rc.y + 8}`,
    },
  ]
})

function select(id) {
  emit('select', id)
}

function onDrop(ev) {
  const kind = ev.dataTransfer.getData('text/plain')
  if (kind) emit('drop', kind)
}
</script>

<style scoped>
.topo-wrap {
  border: 1px solid var(--dasm-border-soft);
  border-radius: 14px;
  background: #f7fbfa;
  overflow: auto;
  max-height: min(76vh, 760px);
}
.topo-wrap.compact {
  max-height: min(72vh, 640px);
}
.topo-svg {
  width: 100%;
  min-height: 420px;
  display: block;
}
.topo-wrap.compact .topo-svg {
  min-height: 280px;
}
.box-ns {
  fill: #e8f4f1;
  stroke: #2f8f7d;
  stroke-width: 2;
  cursor: pointer;
}
.box-inner {
  stroke: #fff;
  stroke-width: 2;
  cursor: pointer;
}
.is-route { fill: #976eb0; }
.is-svc { fill: #2f8f7d; }
.is-pod { fill: #70835a; }
.is-pressure { fill: #3d6b8a; }
.is-authz { fill: #6b4a8a; }
.is-custom { fill: #b07a3d; }
.dim { opacity: 0.45; }
.active {
  stroke: #1d2b36;
  stroke-width: 3;
}
.box-title {
  fill: #12202c;
  font-family: Fraunces, Georgia, serif;
  font-size: 16px;
  font-weight: 700;
}
.box-count {
  fill: #1d2b36;
  font-family: 'Source Sans 3', sans-serif;
  font-size: 15px;
  font-weight: 700;
}
.box-sub {
  fill: #445566;
  font-size: 11px;
  font-family: 'Source Sans 3', sans-serif;
}
.inner-title {
  fill: #fff;
  font-family: Fraunces, Georgia, serif;
  font-size: 16px;
  font-weight: 700;
}
.inner-count,
.inner-sub {
  fill: rgba(255, 255, 255, 0.92);
  font-family: 'Source Sans 3', sans-serif;
}
.inner-count {
  font-size: 15px;
  font-weight: 700;
}
.inner-sub {
  font-size: 11px;
}
.link {
  stroke: #70835a;
  stroke-width: 2;
}
text {
  pointer-events: none;
}
</style>
