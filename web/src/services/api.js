import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30_000,
  withCredentials: true,
})

export default api

export const getVersion = () => api.get('/version').then((r) => r.data)
export const getPlan = () => api.get('/plan').then((r) => r.data)
export const getStatus = () => api.get('/status').then((r) => r.data)
export const getHealth = () => api.get('/health').then((r) => r.data)
export const getReport = () => api.get('/report').then((r) => r.data)
export const getTopology = () => api.get('/topology').then((r) => r.data)
export const putTopology = (body) => api.put('/topology', body).then((r) => r.data)
export const getKubeBurnerPreview = () => api.get('/kube-burner-preview').then((r) => r.data)

export const listTemplates = () => api.get('/templates').then((r) => r.data)
export const saveTemplate = (body) => api.post('/templates', body).then((r) => r.data)
export const selectTemplate = (name) => api.put(`/templates/${encodeURIComponent(name)}`).then((r) => r.data)
export const deleteTemplate = (name) => api.delete(`/templates/${encodeURIComponent(name)}`).then((r) => r.data)

export const getCluster = () => api.get('/cluster').then((r) => r.data)
export const selectCluster = (body) => api.put('/cluster', body).then((r) => r.data)

export const getRun = () => api.get('/runs/current').then((r) => r.data)
export const startRun = (body) => api.post('/runs', body).then((r) => r.data)
export const cancelRun = () => api.post('/runs/cancel').then((r) => r.data)

export async function getAuthConfig() {
  const { data } = await api.get('/auth/config')
  return data
}

export async function getMe() {
  const { data } = await api.get('/auth/me', {
    validateStatus: (s) => (s >= 200 && s < 300) || s === 401,
  })
  if (!data || data.error === 'unauthorized') {
    const err = new Error('unauthorized')
    err.response = { status: 401, data }
    throw err
  }
  return data
}

export function loginUrl() {
  return '/api/v1/auth/login'
}

export function logoutUrl() {
  return '/api/v1/auth/logout'
}
