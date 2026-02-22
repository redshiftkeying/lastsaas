export interface User {
  id: string;
  email: string;
  displayName: string;
  emailVerified: boolean;
  isActive: boolean;
  authMethods: string[];
  createdAt: string;
  updatedAt: string;
  lastLoginAt?: string;
}

export interface MembershipInfo {
  tenantId: string;
  tenantName: string;
  tenantSlug: string;
  role: 'owner' | 'admin' | 'user';
  isRoot: boolean;
}

export interface AuthResponse {
  accessToken: string;
  refreshToken: string;
  user: User;
  memberships: MembershipInfo[];
}

export interface TenantMember {
  userId: string;
  email: string;
  displayName: string;
  role: 'owner' | 'admin' | 'user';
  joinedAt: string;
}

export interface TenantDetail {
  id: string;
  name: string;
  slug: string;
  isRoot: boolean;
  isActive: boolean;
  planId?: string;
  billingWaived: boolean;
  subscriptionCredits: number;
  purchasedCredits: number;
  createdAt: string;
  updatedAt: string;
}

export interface TenantListItem {
  id: string;
  name: string;
  slug: string;
  isRoot: boolean;
  isActive: boolean;
  memberCount: number;
  planName: string;
  billingWaived: boolean;
  subscriptionCredits: number;
  purchasedCredits: number;
  createdAt: string;
}

export interface UserListItem {
  id: string;
  email: string;
  displayName: string;
  emailVerified: boolean;
  isActive: boolean;
  tenantCount: number;
  createdAt: string;
  lastLoginAt?: string;
}

export interface Message {
  id: string;
  userId: string;
  subject: string;
  body: string;
  isSystem: boolean;
  read: boolean;
  createdAt: string;
}

export interface AboutInfo {
  version: string;
  copyright: string;
}

export type LogSeverity = 'critical' | 'high' | 'medium' | 'low' | 'debug';

export interface SystemLog {
  id: string;
  severity: LogSeverity;
  message: string;
  userId?: string;
  createdAt: string;
}

export type ConfigVarType = 'string' | 'numeric' | 'enum' | 'template';

export interface ConfigVar {
  id: string;
  name: string;
  description: string;
  type: ConfigVarType;
  value: string;
  options?: string;
  isSystem: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface EnumOption {
  label: string;
  value: string;
}

export interface UserDetail {
  id: string;
  email: string;
  displayName: string;
  emailVerified: boolean;
  isActive: boolean;
  authMethods: string[];
  createdAt: string;
  updatedAt: string;
  lastLoginAt?: string;
}

export interface UserMembershipDetail {
  tenantId: string;
  tenantName: string;
  tenantSlug: string;
  isRoot: boolean;
  role: 'owner' | 'admin' | 'user';
  joinedAt: string;
  planId: string;
  planName: string;
  billingWaived: boolean;
  subscriptionCredits: number;
  purchasedCredits: number;
}

export interface TenantDeletionInfo {
  tenantId: string;
  tenantName: string;
  isRoot: boolean;
  otherMembers: { userId: string; displayName: string; email: string }[];
}

export interface DeletePreflightResponse {
  canDelete: boolean;
  reason?: string;
  ownerships?: TenantDeletionInfo[];
}

export type EntitlementType = 'bool' | 'numeric';
export type CreditResetPolicy = 'reset' | 'accrue';

export interface EntitlementValue {
  type: EntitlementType;
  boolValue: boolean;
  numericValue: number;
  description: string;
}

export interface Plan {
  id: string;
  name: string;
  description: string;
  monthlyPriceCents: number;
  annualDiscountPct: number;
  usageCreditsPerMonth: number;
  creditResetPolicy: CreditResetPolicy;
  bonusCredits: number;
  userLimit: number;
  entitlements: Record<string, EntitlementValue>;
  isSystem: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface EntitlementKeyInfo {
  key: string;
  type: EntitlementType;
  description: string;
}

export interface PublicPlansResponse {
  plans: Plan[];
  currentPlanId: string;
  billingWaived: boolean;
  tenantSubscriptionCredits: number;
  tenantPurchasedCredits: number;
}

export interface CreditBundle {
  id: string;
  name: string;
  credits: number;
  priceCents: number;
  isActive: boolean;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
}

// --- System Health ---

export interface SystemNode {
  id: string;
  machineId: string;
  hostname: string;
  status: 'active' | 'stale';
  startedAt: string;
  lastSeen: string;
  version: string;
  goVersion: string;
}

export interface CPUMetrics {
  usagePercent: number;
  numCpu: number;
}

export interface MemoryMetrics {
  usedBytes: number;
  totalBytes: number;
  usedPercent: number;
}

export interface DiskMetrics {
  usedBytes: number;
  totalBytes: number;
  usedPercent: number;
}

export interface NetworkMetrics {
  bytesSent: number;
  bytesRecv: number;
}

export interface HTTPMetrics {
  requestCount: number;
  latencyP50: number;
  latencyP95: number;
  latencyP99: number;
  statusCodes: Record<string, number>;
  errorRate4xx: number;
  errorRate5xx: number;
}

export interface MongoMetrics {
  currentConnections: number;
  availableConnections: number;
  dataSizeBytes: number;
  indexSizeBytes: number;
  collections: number;
  opCounters: Record<string, number>;
}

export interface GoRuntimeMetrics {
  numGoroutine: number;
  heapAlloc: number;
  heapSys: number;
  gcPauseNs: number;
  numGC: number;
}

export interface SystemMetric {
  id: string;
  nodeId: string;
  timestamp: string;
  cpu: CPUMetrics;
  memory: MemoryMetrics;
  disk: DiskMetrics;
  network: NetworkMetrics;
  http: HTTPMetrics;
  mongo: MongoMetrics;
  goRuntime: GoRuntimeMetrics;
}

export type TimeRange = '1h' | '6h' | '24h' | '7d' | '30d';
export type NodeFilterMode = 'aggregate' | 'all' | 'single';
