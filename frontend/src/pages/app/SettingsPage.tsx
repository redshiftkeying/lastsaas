import { useEffect, useState } from 'react';
import { Settings, User, KeyRound, CheckCircle, AlertCircle, CreditCard, Receipt, Download, ExternalLink } from 'lucide-react';
import { useAuth } from '../../contexts/AuthContext';
import { authApi, billingApi, plansApi } from '../../api/client';
import type { FinancialTransaction, BillingStatus } from '../../types';
import LoadingSpinner from '../../components/LoadingSpinner';

function InvoiceModal({ tx, tenantName, onClose }: { tx: FinancialTransaction; tenantName: string; onClose: () => void }) {
  const [downloading, setDownloading] = useState(false);

  const handleDownload = async () => {
    setDownloading(true);
    try {
      const blob = await billingApi.getInvoicePDF(tx.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `invoice-${tx.invoiceNumber}.pdf`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      // Error handled by interceptor
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-dark-900 border border-dark-700 rounded-2xl p-6 max-w-lg mx-4 w-full" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold text-white">Invoice</h3>
          <button onClick={onClose} className="text-dark-400 hover:text-white">&times;</button>
        </div>

        <div className="space-y-4 mb-6">
          <div className="flex justify-between">
            <span className="text-dark-400 text-sm">Invoice Number</span>
            <span className="text-white text-sm font-mono">{tx.invoiceNumber}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-dark-400 text-sm">Date</span>
            <span className="text-white text-sm">{new Date(tx.createdAt).toLocaleDateString()}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-dark-400 text-sm">Bill To</span>
            <span className="text-white text-sm">{tenantName}</span>
          </div>
          <hr className="border-dark-800" />
          <div className="flex justify-between">
            <span className="text-dark-300 text-sm">{tx.description}</span>
            <span className="text-white text-sm font-medium">${(tx.amountCents / 100).toFixed(2)}</span>
          </div>
          <hr className="border-dark-800" />
          <div className="flex justify-between">
            <span className="text-white font-semibold">Total</span>
            <span className="text-white font-semibold">${(tx.amountCents / 100).toFixed(2)} {tx.currency.toUpperCase()}</span>
          </div>
        </div>

        <button
          onClick={handleDownload}
          disabled={downloading}
          className="w-full py-2.5 text-sm font-medium bg-primary-500 text-white rounded-lg hover:bg-primary-600 disabled:opacity-60 transition-colors flex items-center justify-center gap-2"
        >
          {downloading ? <LoadingSpinner size="sm" /> : <><Download className="w-4 h-4" /> Download PDF</>}
        </button>
      </div>
    </div>
  );
}

export default function SettingsPage() {
  const { user, refreshUser } = useAuth();
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [passwordSuccess, setPasswordSuccess] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);
  const [tab, setTab] = useState<'profile' | 'billing'>('profile');

  // Billing state
  const [billingStatus, setBillingStatus] = useState<BillingStatus>('none');
  const [billingInterval, setBillingInterval] = useState('');
  const [currentPeriodEnd, setCurrentPeriodEnd] = useState('');
  const [currentPlanName, setCurrentPlanName] = useState('');
  const [tenantName] = useState('');
  const [transactions, setTransactions] = useState<FinancialTransaction[]>([]);
  const [txTotal, setTxTotal] = useState(0);
  const [txPage, setTxPage] = useState(1);
  const [billingLoading, setBillingLoading] = useState(false);
  const [portalLoading, setPortalLoading] = useState(false);
  const [selectedTx, setSelectedTx] = useState<FinancialTransaction | null>(null);

  const loadBillingData = () => {
    setBillingLoading(true);
    Promise.all([
      plansApi.list(),
      billingApi.listTransactions({ page: txPage, perPage: 10 }),
    ])
      .then(([planData, txData]) => {
        setBillingStatus(planData.billingStatus || 'none');
        setBillingInterval(planData.billingInterval || '');
        setCurrentPeriodEnd(planData.currentPeriodEnd || '');
        const plan = planData.plans.find(p => p.id === planData.currentPlanId);
        setCurrentPlanName(plan?.name || 'Free');
        setTransactions(txData.transactions);
        setTxTotal(txData.total);
      })
      .catch(() => {})
      .finally(() => setBillingLoading(false));
  };

  useEffect(() => {
    if (tab === 'billing') {
      loadBillingData();
    }
  }, [tab, txPage]);

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError('');
    setPasswordSuccess('');
    setChangingPassword(true);
    try {
      await authApi.changePassword(currentPassword, newPassword);
      setPasswordSuccess('Password changed successfully');
      setCurrentPassword('');
      setNewPassword('');
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setPasswordError(msg || 'Failed to change password');
    } finally {
      setChangingPassword(false);
    }
  };

  const handleResendVerification = async () => {
    if (!user?.email) return;
    try {
      await authApi.resendVerification(user.email);
      await refreshUser();
    } catch {
      // ignore
    }
  };

  const handlePortal = async () => {
    setPortalLoading(true);
    try {
      const result = await billingApi.portal();
      window.location.href = result.portalUrl;
    } catch {
      // Error handled by interceptor
    } finally {
      setPortalLoading(false);
    }
  };

  const totalPages = Math.ceil(txTotal / 10);

  return (
    <div>
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-white flex items-center gap-3">
          <Settings className="w-7 h-7 text-primary-400" />
          Settings
        </h1>
        <p className="text-dark-400 mt-1">Manage your account</p>
      </div>

      {/* Tab Navigation */}
      <div className="flex gap-1 mb-6 bg-dark-900/50 border border-dark-800 rounded-xl p-1 max-w-xs">
        <button
          onClick={() => setTab('profile')}
          className={`flex-1 px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
            tab === 'profile' ? 'bg-dark-700 text-white' : 'text-dark-400 hover:text-dark-300'
          }`}
        >
          Profile
        </button>
        <button
          onClick={() => setTab('billing')}
          className={`flex-1 px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
            tab === 'billing' ? 'bg-dark-700 text-white' : 'text-dark-400 hover:text-dark-300'
          }`}
        >
          Billing
        </button>
      </div>

      {tab === 'profile' && (
        <div className="space-y-6 max-w-2xl">
          {/* Profile Section */}
          <div className="bg-dark-900/50 backdrop-blur-sm border border-dark-800 rounded-2xl p-6">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2 mb-4">
              <User className="w-5 h-5 text-dark-400" />
              Profile
            </h2>
            <div className="space-y-3">
              <div className="flex items-center justify-between py-2">
                <span className="text-sm text-dark-400">Name</span>
                <span className="text-sm text-white">{user?.displayName}</span>
              </div>
              <div className="flex items-center justify-between py-2 border-t border-dark-800">
                <span className="text-sm text-dark-400">Email</span>
                <span className="text-sm text-white">{user?.email}</span>
              </div>
              <div className="flex items-center justify-between py-2 border-t border-dark-800">
                <span className="text-sm text-dark-400">Email Verified</span>
                <div className="flex items-center gap-2">
                  {user?.emailVerified ? (
                    <span className="flex items-center gap-1 text-sm text-accent-emerald">
                      <CheckCircle className="w-4 h-4" /> Verified
                    </span>
                  ) : (
                    <div className="flex items-center gap-2">
                      <span className="flex items-center gap-1 text-sm text-amber-400">
                        <AlertCircle className="w-4 h-4" /> Not verified
                      </span>
                      <button
                        onClick={handleResendVerification}
                        className="text-xs text-primary-400 hover:text-primary-300 transition-colors"
                      >
                        Resend
                      </button>
                    </div>
                  )}
                </div>
              </div>
              <div className="flex items-center justify-between py-2 border-t border-dark-800">
                <span className="text-sm text-dark-400">Auth Methods</span>
                <div className="flex gap-2">
                  {user?.authMethods.map((method) => (
                    <span key={method} className="px-2 py-0.5 bg-dark-800 rounded text-xs text-dark-300 capitalize">
                      {method}
                    </span>
                  ))}
                </div>
              </div>
            </div>
          </div>

          {/* Change Password Section */}
          <div className="bg-dark-900/50 backdrop-blur-sm border border-dark-800 rounded-2xl p-6">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2 mb-4">
              <KeyRound className="w-5 h-5 text-dark-400" />
              Change Password
            </h2>

            {passwordError && (
              <div className="mb-4 bg-red-500/10 border border-red-500/20 rounded-lg p-3 text-sm text-red-400">{passwordError}</div>
            )}
            {passwordSuccess && (
              <div className="mb-4 bg-accent-emerald/10 border border-accent-emerald/20 rounded-lg p-3 text-sm text-accent-emerald">{passwordSuccess}</div>
            )}

            <form onSubmit={handleChangePassword} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">Current Password</label>
                <input
                  type="password"
                  required
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  className="w-full px-4 py-2.5 bg-dark-800 border border-dark-700 rounded-lg text-white placeholder-dark-500 focus:outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500 transition-colors"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">New Password</label>
                <input
                  type="password"
                  required
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  className="w-full px-4 py-2.5 bg-dark-800 border border-dark-700 rounded-lg text-white placeholder-dark-500 focus:outline-none focus:border-primary-500 focus:ring-1 focus:ring-primary-500 transition-colors"
                  placeholder="Min 10 chars, mixed case, number, special"
                />
              </div>
              <button
                type="submit"
                disabled={changingPassword}
                className="py-2.5 px-6 bg-gradient-to-r from-primary-600 to-primary-500 text-white font-medium rounded-lg hover:from-primary-500 hover:to-primary-400 disabled:opacity-50 disabled:cursor-not-allowed transition-all text-sm"
              >
                {changingPassword ? 'Changing...' : 'Change Password'}
              </button>
            </form>
          </div>
        </div>
      )}

      {tab === 'billing' && (
        <div className="space-y-6 max-w-3xl">
          {billingLoading ? (
            <LoadingSpinner size="lg" className="py-20" />
          ) : (
            <>
              {/* Subscription Summary */}
              <div className="bg-dark-900/50 backdrop-blur-sm border border-dark-800 rounded-2xl p-6">
                <h2 className="text-lg font-semibold text-white flex items-center gap-2 mb-4">
                  <CreditCard className="w-5 h-5 text-dark-400" />
                  Subscription
                </h2>
                <div className="space-y-3">
                  <div className="flex items-center justify-between py-2">
                    <span className="text-sm text-dark-400">Plan</span>
                    <span className="text-sm text-white font-medium">{currentPlanName}</span>
                  </div>
                  <div className="flex items-center justify-between py-2 border-t border-dark-800">
                    <span className="text-sm text-dark-400">Status</span>
                    <span className={`text-sm font-medium ${
                      billingStatus === 'active' ? 'text-accent-emerald' :
                      billingStatus === 'past_due' ? 'text-red-400' :
                      billingStatus === 'canceled' ? 'text-yellow-400' :
                      'text-dark-400'
                    }`}>
                      {billingStatus === 'active' ? 'Active' :
                       billingStatus === 'past_due' ? 'Past Due' :
                       billingStatus === 'canceled' ? 'Canceled' :
                       'None'}
                    </span>
                  </div>
                  {billingInterval && (
                    <div className="flex items-center justify-between py-2 border-t border-dark-800">
                      <span className="text-sm text-dark-400">Billing Interval</span>
                      <span className="text-sm text-white capitalize">{billingInterval}ly</span>
                    </div>
                  )}
                  {currentPeriodEnd && (
                    <div className="flex items-center justify-between py-2 border-t border-dark-800">
                      <span className="text-sm text-dark-400">
                        {billingStatus === 'canceled' ? 'Benefits Until' : 'Next Billing'}
                      </span>
                      <span className="text-sm text-white">{new Date(currentPeriodEnd).toLocaleDateString()}</span>
                    </div>
                  )}
                </div>

                {billingStatus !== 'none' && (
                  <div className="mt-4 pt-4 border-t border-dark-800">
                    <button
                      onClick={handlePortal}
                      disabled={portalLoading}
                      className="inline-flex items-center gap-2 px-4 py-2 text-sm bg-dark-800 text-dark-300 border border-dark-700 rounded-lg hover:border-dark-600 hover:text-white transition-colors disabled:opacity-60"
                    >
                      {portalLoading ? <LoadingSpinner size="sm" /> : <><ExternalLink className="w-4 h-4" /> Update Payment Method</>}
                    </button>
                  </div>
                )}
              </div>

              {/* Transaction History */}
              <div className="bg-dark-900/50 backdrop-blur-sm border border-dark-800 rounded-2xl overflow-hidden">
                <div className="px-6 py-4 border-b border-dark-800">
                  <h2 className="text-lg font-semibold text-white flex items-center gap-2">
                    <Receipt className="w-5 h-5 text-dark-400" />
                    Transaction History
                  </h2>
                </div>

                {transactions.length === 0 ? (
                  <div className="text-center py-12 text-dark-400">No transactions yet</div>
                ) : (
                  <>
                    <div className="overflow-x-auto">
                      <table className="w-full">
                        <thead>
                          <tr className="border-b border-dark-800">
                            <th className="text-left px-6 py-3 text-xs font-medium text-dark-400 uppercase">Date</th>
                            <th className="text-left px-6 py-3 text-xs font-medium text-dark-400 uppercase">Description</th>
                            <th className="text-right px-6 py-3 text-xs font-medium text-dark-400 uppercase">Amount</th>
                            <th className="text-left px-6 py-3 text-xs font-medium text-dark-400 uppercase">Invoice</th>
                            <th className="px-6 py-3"></th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-dark-800/50">
                          {transactions.map(tx => (
                            <tr key={tx.id} className="hover:bg-dark-800/20">
                              <td className="px-6 py-3 text-sm text-dark-300 whitespace-nowrap">
                                {new Date(tx.createdAt).toLocaleDateString()}
                              </td>
                              <td className="px-6 py-3 text-sm text-white">{tx.description}</td>
                              <td className="px-6 py-3 text-sm text-white text-right font-mono">
                                ${(tx.amountCents / 100).toFixed(2)}
                              </td>
                              <td className="px-6 py-3 text-sm text-dark-400 font-mono">{tx.invoiceNumber}</td>
                              <td className="px-6 py-3">
                                <button
                                  onClick={() => setSelectedTx(tx)}
                                  className="text-xs text-primary-400 hover:text-primary-300 transition-colors"
                                >
                                  View
                                </button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>

                    {totalPages > 1 && (
                      <div className="px-6 py-3 border-t border-dark-800 flex items-center justify-between">
                        <span className="text-xs text-dark-400">{txTotal} total</span>
                        <div className="flex gap-2">
                          <button
                            onClick={() => setTxPage(p => Math.max(1, p - 1))}
                            disabled={txPage === 1}
                            className="px-3 py-1 text-xs bg-dark-800 text-dark-300 rounded disabled:opacity-40"
                          >
                            Prev
                          </button>
                          <span className="px-3 py-1 text-xs text-dark-400">{txPage} / {totalPages}</span>
                          <button
                            onClick={() => setTxPage(p => Math.min(totalPages, p + 1))}
                            disabled={txPage === totalPages}
                            className="px-3 py-1 text-xs bg-dark-800 text-dark-300 rounded disabled:opacity-40"
                          >
                            Next
                          </button>
                        </div>
                      </div>
                    )}
                  </>
                )}
              </div>
            </>
          )}

          {selectedTx && (
            <InvoiceModal tx={selectedTx} tenantName={tenantName || 'Your Organization'} onClose={() => setSelectedTx(null)} />
          )}
        </div>
      )}
    </div>
  );
}
