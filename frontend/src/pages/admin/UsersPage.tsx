import { useEffect, useState, useCallback, useRef } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Users, CheckCircle, XCircle, Search, ChevronLeft, ChevronRight, ArrowUpDown } from 'lucide-react';
import { adminApi } from '../../api/client';
import type { UserListItem } from '../../types';
import LoadingSpinner from '../../components/LoadingSpinner';

const PAGE_SIZE = 25;

export default function UsersPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [users, setUsers] = useState<UserListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState(searchParams.get('search') || '');
  const [page, setPage] = useState(Number(searchParams.get('page')) || 1);
  const [sort, setSort] = useState(searchParams.get('sort') || '-createdAt');
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const fetchUsers = useCallback(async (p: number, q: string, s: string) => {
    setLoading(true);
    try {
      const data = await adminApi.listUsers({ page: p, limit: PAGE_SIZE, search: q || undefined, sort: s });
      setUsers(data.users || []);
      setTotal(data.total);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, []);

  // Sync URL params
  useEffect(() => {
    const params: Record<string, string> = {};
    if (page > 1) params.page = String(page);
    if (search) params.search = search;
    if (sort && sort !== '-createdAt') params.sort = sort;
    setSearchParams(params, { replace: true });
  }, [page, search, sort, setSearchParams]);

  // Fetch on page/sort change
  useEffect(() => {
    fetchUsers(page, search, sort);
  }, [page, sort, fetchUsers]); // eslint-disable-line react-hooks/exhaustive-deps

  // Debounced search
  const handleSearchChange = (value: string) => {
    setSearch(value);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setPage(1);
      fetchUsers(1, value, sort);
    }, 300);
  };

  const toggleSort = (field: string) => {
    setSort(prev => prev === field ? `-${field}` : prev === `-${field}` ? field : field);
    setPage(1);
  };

  const toggleStatus = async (user: UserListItem) => {
    try {
      await adminApi.updateUserStatus(user.id, !user.isActive);
      setUsers(prev => prev.map(u => u.id === user.id ? { ...u, isActive: !u.isActive } : u));
    } catch {
      // ignore
    }
  };

  const totalPages = Math.ceil(total / PAGE_SIZE);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white flex items-center gap-3">
            <Users className="w-7 h-7 text-primary-400" />
            Users
          </h1>
          <p className="text-dark-400 mt-1">{total.toLocaleString()} total users</p>
        </div>
      </div>

      {/* Search */}
      <div className="mb-4">
        <div className="relative max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-dark-500" />
          <input
            type="text"
            placeholder="Search by name or email..."
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            className="w-full pl-10 pr-4 py-2.5 bg-dark-800 border border-dark-700 rounded-lg text-white placeholder-dark-500 focus:outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500 transition-colors text-sm"
          />
        </div>
      </div>

      {/* Table */}
      <div className="bg-dark-900/50 backdrop-blur-sm border border-dark-800 rounded-2xl overflow-hidden">
        {loading && users.length === 0 ? (
          <div className="py-20"><LoadingSpinner size="lg" /></div>
        ) : users.length === 0 ? (
          <div className="py-16 text-center text-dark-400">
            {search ? 'No users match your search.' : 'No users yet.'}
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-dark-800">
                    <th className="text-left px-6 py-3.5">
                      <button onClick={() => toggleSort('displayName')} className="flex items-center gap-1.5 text-sm font-medium text-dark-400 hover:text-white transition-colors">
                        User
                        <ArrowUpDown className="w-3 h-3" />
                      </button>
                    </th>
                    <th className="text-left px-6 py-3.5 text-sm font-medium text-dark-400">Verified</th>
                    <th className="text-left px-6 py-3.5 text-sm font-medium text-dark-400">Tenants</th>
                    <th className="text-left px-6 py-3.5">
                      <button onClick={() => toggleSort('createdAt')} className="flex items-center gap-1.5 text-sm font-medium text-dark-400 hover:text-white transition-colors">
                        Joined
                        <ArrowUpDown className="w-3 h-3" />
                      </button>
                    </th>
                    <th className="text-left px-6 py-3.5 text-sm font-medium text-dark-400">Status</th>
                    <th className="text-right px-6 py-3.5 text-sm font-medium text-dark-400">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <tr
                      key={user.id}
                      onClick={() => navigate(`/last/users/${user.id}`)}
                      className="border-b border-dark-800/50 hover:bg-dark-800/30 transition-colors cursor-pointer"
                    >
                      <td className="px-6 py-3.5">
                        <p className="text-sm font-medium text-white">{user.displayName}</p>
                        <p className="text-xs text-dark-500">{user.email}</p>
                      </td>
                      <td className="px-6 py-3.5">
                        {user.emailVerified ? (
                          <CheckCircle className="w-4 h-4 text-accent-emerald" />
                        ) : (
                          <XCircle className="w-4 h-4 text-dark-500" />
                        )}
                      </td>
                      <td className="px-6 py-3.5 text-sm text-dark-300">{user.tenantCount}</td>
                      <td className="px-6 py-3.5 text-sm text-dark-400">
                        {new Date(user.createdAt).toLocaleDateString()}
                      </td>
                      <td className="px-6 py-3.5">
                        <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                          user.isActive
                            ? 'bg-accent-emerald/10 text-accent-emerald'
                            : 'bg-red-500/10 text-red-400'
                        }`}>
                          {user.isActive ? 'Active' : 'Disabled'}
                        </span>
                      </td>
                      <td className="px-6 py-3.5 text-right">
                        <button
                          onClick={(e) => { e.stopPropagation(); toggleStatus(user); }}
                          className={`text-xs px-3 py-1.5 rounded-lg border transition-colors ${
                            user.isActive
                              ? 'border-red-500/30 text-red-400 hover:bg-red-500/10'
                              : 'border-accent-emerald/30 text-accent-emerald hover:bg-accent-emerald/10'
                          }`}
                        >
                          {user.isActive ? 'Disable' : 'Enable'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex items-center justify-between px-6 py-3 border-t border-dark-800">
                <p className="text-sm text-dark-400">
                  Showing {((page - 1) * PAGE_SIZE) + 1}–{Math.min(page * PAGE_SIZE, total)} of {total.toLocaleString()}
                </p>
                <div className="flex items-center gap-1">
                  <button
                    onClick={() => setPage(p => Math.max(1, p - 1))}
                    disabled={page <= 1}
                    className="p-1.5 rounded-lg text-dark-400 hover:text-white hover:bg-dark-800 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                  >
                    <ChevronLeft className="w-4 h-4" />
                  </button>
                  {Array.from({ length: Math.min(totalPages, 7) }, (_, i) => {
                    let p: number;
                    if (totalPages <= 7) {
                      p = i + 1;
                    } else if (page <= 4) {
                      p = i + 1;
                    } else if (page >= totalPages - 3) {
                      p = totalPages - 6 + i;
                    } else {
                      p = page - 3 + i;
                    }
                    return (
                      <button
                        key={p}
                        onClick={() => setPage(p)}
                        className={`min-w-[32px] h-8 rounded-lg text-sm font-medium transition-colors ${
                          p === page
                            ? 'bg-primary-500 text-white'
                            : 'text-dark-400 hover:text-white hover:bg-dark-800'
                        }`}
                      >
                        {p}
                      </button>
                    );
                  })}
                  <button
                    onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                    disabled={page >= totalPages}
                    className="p-1.5 rounded-lg text-dark-400 hover:text-white hover:bg-dark-800 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                  >
                    <ChevronRight className="w-4 h-4" />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
