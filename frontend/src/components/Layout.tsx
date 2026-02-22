import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { LayoutDashboard, Users, Settings, LogOut, Shield, ChevronDown, Bell, CreditCard, Zap } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';
import { useTenant } from '../contexts/TenantContext';
import { messagesApi, plansApi, bundlesApi } from '../api/client';
import { useState, useRef, useEffect } from 'react';

export default function Layout() {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, isAuthenticated, logout, memberships } = useAuth();
  const { activeTenant, setActiveTenant } = useTenant();
  const [showTenantMenu, setShowTenantMenu] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);
  const [showCredits, setShowCredits] = useState(false);
  const [tenantCredits, setTenantCredits] = useState(0);
  const [hasBundles, setHasBundles] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  const isActive = (path: string) => location.pathname === path;

  useEffect(() => {
    if (isAuthenticated) {
      messagesApi.unreadCount()
        .then((data) => setUnreadCount(data.count))
        .catch(() => {});
      plansApi.list()
        .then((data) => {
          const hasCredits = data.plans.some(p => p.usageCreditsPerMonth > 0 || p.bonusCredits > 0);
          setShowCredits(hasCredits);
          setTenantCredits(data.tenantSubscriptionCredits + data.tenantPurchasedCredits);
        })
        .catch(() => {});
      bundlesApi.list()
        .then((data) => setHasBundles(data.bundles.length > 0))
        .catch(() => {});
    }
  }, [isAuthenticated]);

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setShowTenantMenu(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const handleLogout = async () => {
    await logout();
    navigate('/login');
  };

  const navItems = [
    { path: '/dashboard', icon: LayoutDashboard, label: 'Dashboard' },
    { path: '/team', icon: Users, label: 'Team' },
    { path: '/plan', icon: CreditCard, label: 'Plan' },
    { path: '/settings', icon: Settings, label: 'Settings' },
  ];

  return (
    <div className="min-h-screen bg-dark-950">
      {/* Header */}
      <header className="sticky top-0 z-50 bg-dark-900/80 backdrop-blur-xl border-b border-dark-800">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            {/* Logo + Nav */}
            <div className="flex items-center gap-6">
              <Link to="/dashboard" className="flex items-center gap-2">
                <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-primary-500 to-accent-purple flex items-center justify-center">
                  <span className="text-white font-bold text-sm">LS</span>
                </div>
                <span className="font-semibold text-white hidden sm:block">LastSaaS</span>
              </Link>

              {isAuthenticated && (
                <nav className="hidden md:flex items-center gap-1">
                  {navItems.map((item) => (
                    <Link
                      key={item.path}
                      to={item.path}
                      className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors ${
                        isActive(item.path)
                          ? 'bg-primary-500/20 text-primary-400'
                          : 'text-dark-400 hover:text-white hover:bg-dark-800/50'
                      }`}
                    >
                      <item.icon className="w-4 h-4" />
                      <span>{item.label}</span>
                    </Link>
                  ))}
                  {memberships.some(m => m.isRoot) && (
                    <Link
                      to="/last"
                      className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors ${
                        location.pathname.startsWith('/last')
                          ? 'bg-accent-purple/20 text-accent-purple'
                          : 'text-dark-400 hover:text-white hover:bg-dark-800/50'
                      }`}
                    >
                      <Shield className="w-4 h-4" />
                      <span>Admin</span>
                    </Link>
                  )}
                </nav>
              )}
            </div>

            {/* Right side */}
            {isAuthenticated && (
              <div className="flex items-center gap-4">
                {/* Tenant Switcher */}
                {memberships.length > 1 && (
                  <div className="relative" ref={menuRef}>
                    <button
                      onClick={() => setShowTenantMenu(!showTenantMenu)}
                      className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-dark-800 border border-dark-700 text-sm text-dark-300 hover:text-white transition-colors"
                    >
                      <span className="max-w-[120px] truncate">{activeTenant?.tenantName}</span>
                      <ChevronDown className="w-3.5 h-3.5" />
                    </button>
                    {showTenantMenu && (
                      <div className="absolute right-0 mt-2 w-56 bg-dark-800 border border-dark-700 rounded-xl shadow-xl py-1 z-50">
                        {memberships.map((m) => (
                          <button
                            key={m.tenantId}
                            onClick={() => {
                              setActiveTenant(m);
                              setShowTenantMenu(false);
                            }}
                            className={`w-full text-left px-4 py-2.5 text-sm transition-colors ${
                              m.tenantId === activeTenant?.tenantId
                                ? 'bg-primary-500/10 text-primary-400'
                                : 'text-dark-300 hover:bg-dark-700 hover:text-white'
                            }`}
                          >
                            <div className="flex items-center justify-between">
                              <span className="truncate">{m.tenantName}</span>
                              <span className="text-xs text-dark-500 capitalize">{m.role}</span>
                            </div>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                {/* Credits indicator */}
                {showCredits && (
                  <button
                    onClick={() => navigate(hasBundles ? '/buy-credits' : '/plan')}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-dark-800 border border-dark-700 text-sm text-dark-300 hover:text-white hover:border-primary-500/30 transition-colors"
                    title="Usage credits"
                  >
                    <Zap className="w-4 h-4 text-primary-400" />
                    <span className="font-medium">{tenantCredits.toLocaleString()}</span>
                  </button>
                )}

                {/* Messages */}
                <Link
                  to="/messages"
                  className="relative text-dark-400 hover:text-white transition-colors"
                >
                  <Bell className="w-5 h-5" />
                  {unreadCount > 0 && (
                    <span className="absolute -top-1.5 -right-1.5 bg-primary-500 text-white text-[10px] font-medium rounded-full w-4 h-4 flex items-center justify-center">
                      {unreadCount}
                    </span>
                  )}
                </Link>

                {/* User info + Logout */}
                <span className="text-sm text-dark-400 hidden sm:block">{user?.displayName}</span>
                <button
                  onClick={handleLogout}
                  className="flex items-center gap-2 text-dark-400 hover:text-white transition-colors"
                >
                  <LogOut className="w-4 h-4" />
                </button>
              </div>
            )}
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <Outlet context={{ setUnreadCount }} />
      </main>
    </div>
  );
}
