import axios from 'axios';
import type { AuthResponse, TenantMember, TenantDetail, TenantListItem, UserListItem, Message, AboutInfo, SystemLog, ConfigVar, UserDetail, UserMembershipDetail, DeletePreflightResponse, Plan, EntitlementKeyInfo, PublicPlansResponse, CreditBundle, SystemNode, SystemMetric } from '../types';

const api = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
});

// Auth header management
export function setAuthToken(token: string | null) {
  if (token) {
    api.defaults.headers.common['Authorization'] = `Bearer ${token}`;
  } else {
    delete api.defaults.headers.common['Authorization'];
  }
}

export function setTenantHeader(tenantId: string | null) {
  if (tenantId) {
    api.defaults.headers.common['X-Tenant-ID'] = tenantId;
  } else {
    delete api.defaults.headers.common['X-Tenant-ID'];
  }
}

// 401 interceptor
api.interceptors.response.use(
  (res) => res,
  (error) => {
    if (error.response?.status === 401 && !error.config?.url?.includes('/auth/login') && !error.config?.url?.includes('/auth/refresh')) {
      localStorage.removeItem('lastsaas_access_token');
      localStorage.removeItem('lastsaas_refresh_token');
      delete api.defaults.headers.common['Authorization'];
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

// 503 interceptor (system not initialized)
api.interceptors.response.use(
  (res) => res,
  (error) => {
    if (error.response?.status === 503 && error.response?.data?.redirect === '/setup') {
      window.location.href = '/setup';
    }
    return Promise.reject(error);
  }
);

// --- Bootstrap ---
export const bootstrapApi = {
  status: () => api.get<{ initialized: boolean }>('/bootstrap/status').then(r => r.data),
};

// --- Auth ---
export const authApi = {
  register: (data: { email: string; password: string; displayName: string; invitationToken?: string }) =>
    api.post<AuthResponse>('/auth/register', data).then(r => r.data),
  login: (data: { email: string; password: string }) =>
    api.post<AuthResponse>('/auth/login', data).then(r => r.data),
  logout: (refreshToken?: string) =>
    api.post('/auth/logout', { refreshToken }),
  refresh: (refreshToken: string) =>
    api.post<AuthResponse>('/auth/refresh', { refreshToken }).then(r => r.data),
  getMe: () =>
    api.get<{ user: import('../types').User; memberships: import('../types').MembershipInfo[] }>('/auth/me').then(r => r.data),
  verifyEmail: (token: string) =>
    api.post('/auth/verify-email', { token }).then(r => r.data),
  resendVerification: (email: string) =>
    api.post('/auth/resend-verification', { email }).then(r => r.data),
  forgotPassword: (email: string) =>
    api.post('/auth/forgot-password', { email }).then(r => r.data),
  resetPassword: (token: string, newPassword: string) =>
    api.post('/auth/reset-password', { token, newPassword }).then(r => r.data),
  changePassword: (currentPassword: string, newPassword: string) =>
    api.post('/auth/change-password', { currentPassword, newPassword }).then(r => r.data),
  acceptInvitation: (token: string) =>
    api.post('/auth/accept-invitation', { token }).then(r => r.data),
};

// --- Tenant ---
export const tenantApi = {
  listMembers: () =>
    api.get<{ members: TenantMember[] }>('/tenant/members').then(r => r.data),
  inviteMember: (email: string, role: string) =>
    api.post('/tenant/members/invite', { email, role }).then(r => r.data),
  removeMember: (userId: string) =>
    api.delete(`/tenant/members/${userId}`).then(r => r.data),
  changeRole: (userId: string, role: string) =>
    api.patch(`/tenant/members/${userId}/role`, { role }).then(r => r.data),
  transferOwnership: (userId: string) =>
    api.post(`/tenant/members/${userId}/transfer-ownership`).then(r => r.data),
};

// --- Messages ---
export const messagesApi = {
  list: () =>
    api.get<{ messages: Message[] }>('/messages').then(r => r.data),
  unreadCount: () =>
    api.get<{ count: number }>('/messages/unread-count').then(r => r.data),
  markRead: (id: string) =>
    api.patch(`/messages/${id}/read`).then(r => r.data),
};

// --- Admin ---
export const adminApi = {
  getAbout: () =>
    api.get<AboutInfo>('/admin/about').then(r => r.data),
  listTenants: () =>
    api.get<{ tenants: TenantListItem[] }>('/admin/tenants').then(r => r.data),
  getTenant: (id: string) =>
    api.get<{ tenant: TenantDetail; members: TenantMember[] }>(`/admin/tenants/${id}`).then(r => r.data),
  updateTenant: (id: string, data: { name?: string; billingWaived?: boolean; subscriptionCredits?: number; purchasedCredits?: number }) =>
    api.put(`/admin/tenants/${id}`, data).then(r => r.data),
  updateTenantStatus: (id: string, isActive: boolean) =>
    api.patch(`/admin/tenants/${id}/status`, { isActive }).then(r => r.data),
  listUsers: () =>
    api.get<{ users: UserListItem[] }>('/admin/users').then(r => r.data),
  updateUserStatus: (id: string, isActive: boolean) =>
    api.patch(`/admin/users/${id}/status`, { isActive }).then(r => r.data),
  listLogs: (params?: { page?: number; perPage?: number; severity?: string; search?: string; userId?: string }) =>
    api.get<{ logs: SystemLog[]; total: number }>('/admin/logs', { params }).then(r => r.data),
  listConfig: () =>
    api.get<{ configs: ConfigVar[] }>('/admin/config').then(r => r.data),
  getConfig: (name: string) =>
    api.get<ConfigVar>(`/admin/config/${name}`).then(r => r.data),
  updateConfig: (name: string, value: string, opts?: { description?: string; options?: string }) =>
    api.put<ConfigVar>(`/admin/config/${name}`, { value, ...opts }).then(r => r.data),
  createConfig: (data: { name: string; description: string; type: string; value: string; options?: string }) =>
    api.post<ConfigVar>('/admin/config', data).then(r => r.data),
  deleteConfig: (name: string) =>
    api.delete(`/admin/config/${name}`).then(r => r.data),
  getUser: (id: string) =>
    api.get<{ user: UserDetail; memberships: UserMembershipDetail[] }>(`/admin/users/${id}`).then(r => r.data),
  updateUser: (id: string, data: { email?: string; displayName?: string }) =>
    api.put(`/admin/users/${id}`, data).then(r => r.data),
  updateUserRole: (userId: string, tenantId: string, role: string) =>
    api.patch(`/admin/users/${userId}/role/${tenantId}`, { role }).then(r => r.data),
  preflightDeleteUser: (id: string) =>
    api.get<DeletePreflightResponse>(`/admin/users/${id}/preflight-delete`).then(r => r.data),
  deleteUser: (id: string, data?: { replacementOwners?: Record<string, string>; confirmTenantDeletions?: string[] }) =>
    api.delete(`/admin/users/${id}`, { data }).then(r => r.data),
  listPlans: () =>
    api.get<{ plans: Plan[] }>('/admin/plans').then(r => r.data),
  getPlan: (id: string) =>
    api.get<Plan>(`/admin/plans/${id}`).then(r => r.data),
  createPlan: (data: Omit<Plan, 'id' | 'isSystem' | 'createdAt' | 'updatedAt'>) =>
    api.post<Plan>('/admin/plans', data).then(r => r.data),
  updatePlan: (id: string, data: Omit<Plan, 'id' | 'isSystem' | 'createdAt' | 'updatedAt'>) =>
    api.put<Plan>(`/admin/plans/${id}`, data).then(r => r.data),
  deletePlan: (id: string) =>
    api.delete(`/admin/plans/${id}`).then(r => r.data),
  listEntitlementKeys: () =>
    api.get<{ keys: EntitlementKeyInfo[] }>('/admin/entitlement-keys').then(r => r.data),
  assignTenantPlan: (tenantId: string, planId: string | null, billingWaived?: boolean) =>
    api.patch(`/admin/tenants/${tenantId}/plan`, { planId, billingWaived }).then(r => r.data),
  listBundles: () =>
    api.get<{ bundles: CreditBundle[] }>('/admin/credit-bundles').then(r => r.data),
  createBundle: (data: Omit<CreditBundle, 'id' | 'createdAt' | 'updatedAt'>) =>
    api.post<CreditBundle>('/admin/credit-bundles', data).then(r => r.data),
  updateBundle: (id: string, data: Omit<CreditBundle, 'id' | 'createdAt' | 'updatedAt'>) =>
    api.put<CreditBundle>(`/admin/credit-bundles/${id}`, data).then(r => r.data),
  deleteBundle: (id: string) =>
    api.delete(`/admin/credit-bundles/${id}`).then(r => r.data),
  listHealthNodes: () =>
    api.get<{ nodes: SystemNode[] }>('/admin/health/nodes').then(r => r.data),
  getHealthMetrics: (params?: { node?: string; range?: string }) =>
    api.get<{ metrics: SystemMetric[]; from: string; to: string }>('/admin/health/metrics', { params }).then(r => r.data),
  getHealthCurrent: () =>
    api.get<{ metrics: SystemMetric[] }>('/admin/health/current').then(r => r.data),
};

// --- Plans (public, authenticated) ---
export const plansApi = {
  list: () =>
    api.get<PublicPlansResponse>('/plans').then(r => r.data),
};

// --- Credit Bundles (public, authenticated) ---
export const bundlesApi = {
  list: () =>
    api.get<{ bundles: CreditBundle[] }>('/credit-bundles').then(r => r.data),
};

export default api;
