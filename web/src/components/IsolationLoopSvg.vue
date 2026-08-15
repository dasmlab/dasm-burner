<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as d3 from 'd3'

const props = defineProps({
  steps: { type: Array, default: () => [] },
  causality: { type: Array, default: () => [] },
})

const host = ref(null)
let svg

function draw() {
  const el = host.value
  if (!el) return
  d3.select(el).selectAll('*').remove()
  const W = 960
  const H = 420
  svg = d3.select(el).append('svg')
    .attr('viewBox', `0 0 ${W} ${H}`)
    .attr('class', 'iso-svg')
    .attr('role', 'img')
    .attr('aria-label', 'Isolated wave closed loop: baseline, apply, settle, delete, give-back, reset')

  const cx = 280
  const cy = 210
  const R = 148
  const steps = props.steps.length ? props.steps : []
  const n = steps.length || 1
  const nodes = steps.map((s, i) => {
    const a = -Math.PI / 2 + (i * 2 * Math.PI) / n
    return { ...s, x: cx + Math.cos(a) * R, y: cy + Math.sin(a) * R }
  })

  const g = svg.append('g')
  const defs = svg.append('defs')
  defs.append('marker').attr('id', 'iso-arrow').attr('viewBox', '0 0 10 10')
    .attr('refX', 9).attr('refY', 5).attr('markerWidth', 7).attr('markerHeight', 7).attr('orient', 'auto')
    .append('path').attr('d', 'M 0 0 L 10 5 L 0 10 z').attr('fill', '#3f7a6b')

  for (let i = 0; i < nodes.length; i++) {
    const a = nodes[i]
    const b = nodes[(i + 1) % nodes.length]
    g.append('line')
      .attr('x1', a.x).attr('y1', a.y).attr('x2', b.x).attr('y2', b.y)
      .attr('stroke', '#3f7a6b').attr('stroke-width', 2)
      .attr('marker-end', 'url(#iso-arrow)')
  }

  const box = g.selectAll('g.step').data(nodes).enter().append('g').attr('class', 'step')
    .attr('transform', (d) => `translate(${d.x},${d.y})`)
  box.append('rect')
    .attr('x', -78).attr('y', -28).attr('width', 156).attr('height', 56).attr('rx', 10)
    .attr('fill', '#fff').attr('stroke', '#3f7a6b').attr('stroke-width', 1.6)
  box.append('text').attr('text-anchor', 'middle').attr('y', -4).attr('fill', '#1d2b36')
    .attr('font-size', 13).attr('font-weight', 700)
    .text((d) => d.title)
  box.append('text').attr('text-anchor', 'middle').attr('y', 14).attr('fill', '#607483')
    .attr('font-size', 10)
    .text((d) => d.id)

  g.append('circle').attr('cx', cx).attr('cy', cy).attr('r', 46).attr('fill', '#1f6f62')
  g.append('text').attr('x', cx).attr('y', cy - 4).attr('text-anchor', 'middle').attr('fill', '#fff')
    .attr('font-size', 12).attr('font-weight', 700).text('wave k')
  g.append('text').attr('x', cx).attr('y', cy + 14).attr('text-anchor', 'middle').attr('fill', '#d7efe9')
    .attr('font-size', 10).text('then recover')

  // leftover RSS vs objects (the finding)
  const chart = svg.append('g').attr('transform', 'translate(560,70)')
  chart.append('text').attr('fill', '#1d2b36').attr('font-size', 13).attr('font-weight', 700)
    .text('What we measure')
  const xs = d3.scaleLinear().domain([0, 5]).range([0, 340])
  const ys = d3.scaleLinear().domain([0, 1]).range([140, 20])
  const pods = [0.05, 0.55, 0.55, 0.08, 0.05, 0.05]
  const rss = [0.22, 0.62, 0.78, 0.74, 0.70, 0.68]
  const line = d3.line().x((_, i) => xs(i)).y((d) => ys(d)).curve(d3.curveMonotoneX)
  chart.append('path').attr('d', line(pods)).attr('fill', 'none').attr('stroke', '#8e6b3a').attr('stroke-width', 2).attr('stroke-dasharray', '5 4')
  chart.append('path').attr('d', line(rss)).attr('fill', 'none').attr('stroke', '#c0392b').attr('stroke-width', 2.4)
  chart.append('text').attr('x', 0).attr('y', 168).attr('fill', '#c0392b').attr('font-size', 11).text('kas RSS (stays high)')
  chart.append('text').attr('x', 160).attr('y', 168).attr('fill', '#8e6b3a').attr('font-size', 11).text('objects (gone after delete)')
  chart.append('text').attr('x', 0).attr('y', 188).attr('fill', '#607483').attr('font-size', 11)
    .text('Give-back gap = ETCD008. Restart kas to cold-reset.')

  const chain = props.causality || []
  const cg = svg.append('g').attr('transform', 'translate(40,388)')
  cg.append('text').attr('fill', '#607483').attr('font-size', 11).attr('font-weight', 700).text('Causality')
  chain.forEach((c, i) => {
    cg.append('text').attr('x', 78 + i * 145).attr('y', 0).attr('fill', '#1d2b36').attr('font-size', 11).text(c)
    if (i < chain.length - 1) {
      cg.append('text').attr('x', 78 + i * 145 + 118).attr('y', 0).attr('fill', '#3f7a6b').attr('font-size', 11).text('→')
    }
  })
}

onMounted(draw)
watch(() => [props.steps, props.causality], draw, { deep: true })
onBeforeUnmount(() => { if (host.value) d3.select(host.value).selectAll('*').remove() })
</script>

<template>
  <div ref="host" class="iso-host" />
</template>

<style scoped>
.iso-host { width: 100%; }
.iso-host :deep(.iso-svg) { width: 100%; height: auto; display: block; }
</style>
