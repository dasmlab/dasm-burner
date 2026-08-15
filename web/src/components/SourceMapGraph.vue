<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as d3 from 'd3'

const props = defineProps({
  cluster: { type: String, default: '' },
  openshift: { type: String, default: '' },
  pieces: { type: Array, default: () => [] },
})
const emit = defineEmits(['select'])
const host = ref(null)

function draw() {
  const el = host.value
  if (!el) return
  d3.select(el).selectAll('*').remove()
  const W = 960
  const H = 420
  const svg = d3.select(el).append('svg')
    .attr('viewBox', `0 0 ${W} ${H}`)
    .attr('class', 'src-svg')
    .attr('role', 'img')
    .attr('aria-label', 'Control-plane source map: cluster to kube-apiserver, etcd, ovn-kube, oauth-apiserver')

  const nodes = [
    { id: 'cluster', name: props.cluster || 'cluster', kind: 'cluster', sha: props.openshift || '' },
    ...props.pieces.map((p) => ({
      id: p.id,
      name: p.name,
      kind: 'piece',
      sha: (p.payloadSha || '').slice(0, 8),
      piece: p,
    })),
  ]
  const links = props.pieces.map((p) => ({ source: 'cluster', target: p.id }))

  const sim = d3.forceSimulation(nodes)
    .force('link', d3.forceLink(links).id((d) => d.id).distance(160))
    .force('charge', d3.forceManyBody().strength(-420))
    .force('center', d3.forceCenter(W / 2, H / 2))
    .force('collide', d3.forceCollide(70))
    .stop()
  for (let i = 0; i < 80; i++) sim.tick()

  const g = svg.append('g')
  g.selectAll('line').data(links).enter().append('line')
    .attr('x1', (d) => d.source.x).attr('y1', (d) => d.source.y)
    .attr('x2', (d) => d.target.x).attr('y2', (d) => d.target.y)
    .attr('stroke', '#9e73b2').attr('stroke-width', 1.6)

  const ng = g.selectAll('g.node').data(nodes).enter().append('g')
    .attr('class', 'node')
    .attr('transform', (d) => `translate(${d.x},${d.y})`)
    .style('cursor', (d) => (d.kind === 'piece' ? 'pointer' : 'default'))
    .on('click', (_, d) => { if (d.piece) emit('select', d.piece) })

  ng.append('circle')
    .attr('r', (d) => (d.kind === 'cluster' ? 46 : 38))
    .attr('fill', (d) => (d.kind === 'cluster' ? '#1f6f62' : '#fff'))
    .attr('stroke', (d) => (d.kind === 'cluster' ? '#1f6f62' : '#9e73b2'))
    .attr('stroke-width', 2)
  ng.append('text').attr('text-anchor', 'middle').attr('y', -4)
    .attr('fill', (d) => (d.kind === 'cluster' ? '#fff' : '#1d2b36'))
    .attr('font-size', 12).attr('font-weight', 700)
    .text((d) => d.name)
  ng.append('text').attr('text-anchor', 'middle').attr('y', 14)
    .attr('fill', (d) => (d.kind === 'cluster' ? '#d7efe9' : '#607483'))
    .attr('font-size', 10)
    .text((d) => d.sha)
}

onMounted(draw)
watch(() => [props.cluster, props.openshift, props.pieces], draw, { deep: true })
onBeforeUnmount(() => { if (host.value) d3.select(host.value).selectAll('*').remove() })
</script>

<template>
  <div ref="host" class="src-host" />
</template>

<style scoped>
.src-host { width: 100%; }
.src-host :deep(.src-svg) { width: 100%; height: auto; display: block; }
</style>
