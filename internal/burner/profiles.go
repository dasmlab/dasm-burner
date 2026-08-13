package burner

const metricsProfile = `- query: sum(rate(apiserver_request_total[2m]))
  metricName: apiRequestRate
- query: histogram_quantile(0.99, sum(rate(apiserver_request_duration_seconds_bucket[5m])) by (le))
  metricName: apiLatencyP99
- query: histogram_quantile(0.99, sum(rate(etcd_request_duration_seconds_bucket[5m])) by (le))
  metricName: etcdRequestP99
- query: histogram_quantile(0.99, rate(etcd_disk_wal_fsync_duration_seconds_bucket[5m]))
  metricName: etcdWalFsyncP99
- query: sum(rate(apiserver_request_total{code=~"5.."}[2m]))
  metricName: apiErrorRate
- query: avg(container_memory_working_set_bytes{namespace="openshift-ovn-kubernetes",container!="",container!="POD"})
  metricName: ovnKubeNodeMemoryAvg
- query: avg(rate(container_cpu_usage_seconds_total{namespace="openshift-ovn-kubernetes",container!="",container!="POD"}[2m]))
  metricName: ovnKubeNodeCPUAvg
- query: avg(container_memory_working_set_bytes{namespace="openshift-ovn-kubernetes",container="ovn-controller"})
  metricName: ovsDaemonsMemoryAvg
- query: kube_node_status_condition{condition="Ready",status="true"}
  metricName: nodesReady
  instant: true
`

const alertsProfile = `- expr: sum(rate(apiserver_request_total{code=~"5.."}[2m])) > 1
  severity: warning
  description: API server 5xx rate is elevated
- expr: histogram_quantile(0.99, sum(rate(apiserver_request_duration_seconds_bucket[5m])) by (le)) > 1
  severity: warning
  description: API server p99 latency is above 1s
- expr: histogram_quantile(0.99, rate(etcd_disk_wal_fsync_duration_seconds_bucket[5m])) > 0.5
  severity: warning
  description: etcd WAL fsync p99 is above 500ms
`
