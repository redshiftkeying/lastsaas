import { useEffect, useState, useCallback } from 'react';
import { Tag, Plus, X, Ban, Calendar, Filter } from 'lucide-react';
import { toast } from 'sonner';
import { adminApi } from '../../api/client';
import { getErrorMessage } from '../../utils/errors';
import type { Promotion, EligibleProduct } from '../../types';
import TableSkeleton from '../../components/TableSkeleton';
import ConfirmModal from '../../components/ConfirmModal';

export default function PromotionsPage() {
  const [promotions, setPromotions] = useState<Promotion[]>([]);
  const [productNames, setProductNames] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [deactivateTarget, setDeactivateTarget] = useState<Promotion | null>(null);
  const [deactivating, setDeactivating] = useState(false);

  const fetchPromotions = useCallback(async () => {
    try {
      const data = await adminApi.listPromotions();
      setPromotions(data.promotions);
      setProductNames(data.productNames || {});
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchPromotions(); }, [fetchPromotions]);

  const handleDeactivate = async () => {
    if (!deactivateTarget) return;
    setDeactivating(true);
    try {
      await adminApi.deactivatePromotion(deactivateTarget.id);
      toast.success(`${deactivateTarget.code} deactivated`);
      setDeactivateTarget(null);
      fetchPromotions();
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setDeactivating(false);
    }
  };

  const formatExpiry = (ts: number) => {
    if (!ts) return null;
    const d = new Date(ts * 1000);
    const now = new Date();
    const isExpired = d < now;
    return (
      <span className={isExpired ? 'text-red-400' : 'text-dark-400'}>
        {isExpired ? 'Expired ' : 'Expires '}{d.toLocaleDateString()}
      </span>
    );
  };

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white flex items-center gap-3">
            <Tag className="w-7 h-7 text-primary-400" />
            Promotions
          </h1>
          <p className="text-dark-400 mt-1">Manage Stripe promotion codes and coupons</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 px-4 py-2 bg-primary-500 text-white text-sm font-medium rounded-lg hover:bg-primary-600 transition-colors"
        >
          <Plus className="w-4 h-4" />
          Create Code
        </button>
      </div>

      {loading ? (
        <div className="bg-dark-900/50 border border-dark-800 rounded-2xl overflow-hidden">
          <TableSkeleton rows={6} cols={6} />
        </div>
      ) : (
        <div className="bg-dark-900/50 border border-dark-800 rounded-2xl overflow-hidden">
          {promotions.length === 0 ? (
            <div className="text-center py-12 text-dark-400">No promotion codes yet</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-dark-800">
                    <th className="text-left px-6 py-3 text-xs font-medium text-dark-400 uppercase">Code</th>
                    <th className="text-left px-6 py-3 text-xs font-medium text-dark-400 uppercase">Discount</th>
                    <th className="text-left px-6 py-3 text-xs font-medium text-dark-400 uppercase">Status</th>
                    <th className="text-left px-6 py-3 text-xs font-medium text-dark-400 uppercase">Applies To</th>
                    <th className="text-right px-6 py-3 text-xs font-medium text-dark-400 uppercase">Redemptions</th>
                    <th className="text-left px-6 py-3 text-xs font-medium text-dark-400 uppercase">Expiry</th>
                    <th className="text-right px-6 py-3 text-xs font-medium text-dark-400 uppercase">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-dark-800/50">
                  {promotions.map(promo => (
                    <tr key={promo.id} className="hover:bg-dark-800/20">
                      <td className="px-6 py-3 text-sm text-white font-mono font-medium">{promo.code}</td>
                      <td className="px-6 py-3 text-sm text-dark-300">
                        {promo.percentOff > 0
                          ? `${promo.percentOff}% off`
                          : `${(promo.amountOff / 100).toFixed(2)} ${(promo.currency || 'usd').toUpperCase()} off`
                        }
                      </td>
                      <td className="px-6 py-3">
                        <span className={`px-2 py-0.5 text-xs font-medium rounded-full ${
                          promo.active
                            ? 'bg-accent-emerald/10 text-accent-emerald'
                            : 'bg-dark-700 text-dark-400'
                        }`}>
                          {promo.active ? 'Active' : 'Inactive'}
                        </span>
                      </td>
                      <td className="px-6 py-3 text-sm">
                        {promo.appliesToProducts && promo.appliesToProducts.length > 0 ? (
                          <div className="flex flex-wrap gap-1">
                            {promo.appliesToProducts.map(pid => (
                              <span key={pid} className="px-1.5 py-0.5 text-xs bg-dark-700 text-dark-300 rounded">
                                {productNames[pid] || pid.slice(0, 12) + '...'}
                              </span>
                            ))}
                          </div>
                        ) : (
                          <span className="text-dark-500 text-xs">All products</span>
                        )}
                      </td>
                      <td className="px-6 py-3 text-sm text-dark-300 text-right font-mono">
                        {promo.timesRedeemed}
                        {promo.maxRedemptions > 0 && ` / ${promo.maxRedemptions}`}
                      </td>
                      <td className="px-6 py-3 text-sm whitespace-nowrap">
                        {promo.expiresAt ? formatExpiry(promo.expiresAt) : (
                          <span className="text-dark-500 text-xs">Never</span>
                        )}
                      </td>
                      <td className="px-6 py-3 text-right">
                        {promo.active && (
                          <button
                            onClick={() => setDeactivateTarget(promo)}
                            className="p-1.5 text-dark-400 hover:text-red-400 transition-colors"
                            aria-label="Deactivate promotion"
                          >
                            <Ban className="w-4 h-4" />
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {showCreate && (
        <CreatePromotionModal
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); fetchPromotions(); }}
        />
      )}

      <ConfirmModal
        open={deactivateTarget !== null}
        onClose={() => setDeactivateTarget(null)}
        onConfirm={handleDeactivate}
        title="Deactivate Promotion"
        message={`Are you sure you want to deactivate the promotion code "${deactivateTarget?.code}"? It will no longer be usable at checkout.`}
        confirmLabel="Deactivate"
        confirmVariant="danger"
        loading={deactivating}
      />
    </div>
  );
}

function CreatePromotionModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [code, setCode] = useState('');
  const [name, setName] = useState('');
  const [discountType, setDiscountType] = useState<'percent' | 'amount'>('percent');
  const [percentOff, setPercentOff] = useState('10');
  const [amountOff, setAmountOff] = useState('5.00');
  const [maxRedemptions, setMaxRedemptions] = useState('');
  const [expiresAt, setExpiresAt] = useState('');
  const [restrictProducts, setRestrictProducts] = useState(false);
  const [selectedProducts, setSelectedProducts] = useState<Set<string>>(new Set());
  const [eligibleProducts, setEligibleProducts] = useState<EligibleProduct[]>([]);
  const [loadingProducts, setLoadingProducts] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // Fetch eligible products when restriction toggle is enabled.
  useEffect(() => {
    if (restrictProducts && eligibleProducts.length === 0) {
      setLoadingProducts(true);
      adminApi.listEligibleProducts()
        .then(data => setEligibleProducts(data.items))
        .catch(err => toast.error(getErrorMessage(err)))
        .finally(() => setLoadingProducts(false));
    }
  }, [restrictProducts, eligibleProducts.length]);

  const toggleProduct = (id: string) => {
    setSelectedProducts(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleSave = async () => {
    if (!code.trim()) {
      setError('Code is required');
      return;
    }
    if (restrictProducts && selectedProducts.size === 0) {
      setError('Select at least one plan or bundle, or disable product restrictions');
      return;
    }
    setSaving(true);
    setError('');
    try {
      const appliesTo = restrictProducts
        ? eligibleProducts
            .filter(p => selectedProducts.has(p.id))
            .map(p => ({ type: p.type, id: p.id }))
        : undefined;

      await adminApi.createPromotion({
        code: code.trim().toUpperCase(),
        name: name.trim() || undefined,
        percentOff: discountType === 'percent' ? parseFloat(percentOff) || 0 : undefined,
        amountOff: discountType === 'amount' ? Math.round((parseFloat(amountOff) || 0) * 100) : undefined,
        maxRedemptions: maxRedemptions ? parseInt(maxRedemptions) : undefined,
        expiresAt: expiresAt || undefined,
        appliesTo,
      });
      toast.success(`Promotion code ${code.trim().toUpperCase()} created`);
      onCreated();
    } catch (err: any) {
      setError(err.response?.data?.error || getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const plans = eligibleProducts.filter(p => p.type === 'plan');
  const bundles = eligibleProducts.filter(p => p.type === 'bundle');

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="fixed inset-0 bg-black/60 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-dark-900 rounded-2xl border border-dark-700 p-6 w-full max-w-lg max-h-[90vh] overflow-y-auto" role="dialog" aria-modal="true">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold text-white">Create Promotion Code</h3>
          <button onClick={onClose} className="p-2 text-dark-400 hover:text-white transition-colors" aria-label="Close">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-dark-300 mb-1">Code</label>
            <input
              value={code}
              onChange={e => setCode(e.target.value.toUpperCase())}
              placeholder="e.g. SAVE20"
              className="w-full px-3 py-2 bg-dark-800 border border-dark-700 rounded-lg text-white text-sm font-mono focus:border-primary-500 focus:outline-none"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-dark-300 mb-1">Name (optional)</label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="Display name for the coupon"
              className="w-full px-3 py-2 bg-dark-800 border border-dark-700 rounded-lg text-white text-sm focus:border-primary-500 focus:outline-none"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-dark-300 mb-2">Discount Type</label>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={() => setDiscountType('percent')}
                className={`flex-1 px-3 py-2 rounded-lg border text-sm font-medium transition-colors ${
                  discountType === 'percent' ? 'bg-primary-500/20 border-primary-500/50 text-primary-400' : 'bg-dark-800 border-dark-700 text-dark-400'
                }`}
              >
                Percentage
              </button>
              <button
                type="button"
                onClick={() => setDiscountType('amount')}
                className={`flex-1 px-3 py-2 rounded-lg border text-sm font-medium transition-colors ${
                  discountType === 'amount' ? 'bg-primary-500/20 border-primary-500/50 text-primary-400' : 'bg-dark-800 border-dark-700 text-dark-400'
                }`}
              >
                Fixed Amount
              </button>
            </div>
          </div>

          {discountType === 'percent' ? (
            <div>
              <label className="block text-sm font-medium text-dark-300 mb-1">Percent Off</label>
              <input
                type="text"
                inputMode="decimal"
                value={percentOff}
                onChange={e => setPercentOff(e.target.value)}
                className="w-full px-3 py-2 bg-dark-800 border border-dark-700 rounded-lg text-white text-sm focus:border-primary-500 focus:outline-none"
              />
            </div>
          ) : (
            <div>
              <label className="block text-sm font-medium text-dark-300 mb-1">Amount Off ($)</label>
              <input
                type="text"
                inputMode="decimal"
                value={amountOff}
                onChange={e => setAmountOff(e.target.value)}
                className="w-full px-3 py-2 bg-dark-800 border border-dark-700 rounded-lg text-white text-sm focus:border-primary-500 focus:outline-none"
              />
            </div>
          )}

          <div className="flex gap-4">
            <div className="flex-1">
              <label className="block text-sm font-medium text-dark-300 mb-1">Max Redemptions</label>
              <input
                type="text"
                inputMode="numeric"
                value={maxRedemptions}
                onChange={e => setMaxRedemptions(e.target.value)}
                placeholder="Unlimited"
                className="w-full px-3 py-2 bg-dark-800 border border-dark-700 rounded-lg text-white text-sm focus:border-primary-500 focus:outline-none"
              />
            </div>
            <div className="flex-1">
              <label className="block text-sm font-medium text-dark-300 mb-1 flex items-center gap-1.5">
                <Calendar className="w-3.5 h-3.5" />
                Expiration Date
              </label>
              <input
                type="date"
                value={expiresAt}
                onChange={e => setExpiresAt(e.target.value)}
                min={new Date().toISOString().split('T')[0]}
                className="w-full px-3 py-2 bg-dark-800 border border-dark-700 rounded-lg text-white text-sm focus:border-primary-500 focus:outline-none [color-scheme:dark]"
              />
            </div>
          </div>

          {/* Product restrictions */}
          <div className="border-t border-dark-800 pt-4">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={restrictProducts}
                onChange={e => setRestrictProducts(e.target.checked)}
                className="w-4 h-4 rounded border-dark-600 bg-dark-800 text-primary-500 focus:ring-primary-500"
              />
              <Filter className="w-4 h-4 text-dark-400" />
              <span className="text-sm font-medium text-dark-300">Restrict to specific plans or credit bundles</span>
            </label>

            {restrictProducts && (
              <div className="mt-3 space-y-3">
                {loadingProducts ? (
                  <p className="text-xs text-dark-500">Loading products...</p>
                ) : eligibleProducts.length === 0 ? (
                  <p className="text-xs text-dark-500">No paid plans or credit bundles configured</p>
                ) : (
                  <>
                    {plans.length > 0 && (
                      <div>
                        <p className="text-xs font-medium text-dark-400 uppercase tracking-wide mb-2">Plans</p>
                        <div className="space-y-1.5">
                          {plans.map(p => (
                            <label key={p.id} className="flex items-center gap-2.5 px-3 py-2 rounded-lg bg-dark-800/50 border border-dark-700/50 cursor-pointer hover:border-dark-600 transition-colors">
                              <input
                                type="checkbox"
                                checked={selectedProducts.has(p.id)}
                                onChange={() => toggleProduct(p.id)}
                                className="w-4 h-4 rounded border-dark-600 bg-dark-800 text-primary-500 focus:ring-primary-500"
                              />
                              <span className="text-sm text-white">{p.name}</span>
                            </label>
                          ))}
                        </div>
                      </div>
                    )}
                    {bundles.length > 0 && (
                      <div>
                        <p className="text-xs font-medium text-dark-400 uppercase tracking-wide mb-2">Credit Bundles</p>
                        <div className="space-y-1.5">
                          {bundles.map(b => (
                            <label key={b.id} className="flex items-center gap-2.5 px-3 py-2 rounded-lg bg-dark-800/50 border border-dark-700/50 cursor-pointer hover:border-dark-600 transition-colors">
                              <input
                                type="checkbox"
                                checked={selectedProducts.has(b.id)}
                                onChange={() => toggleProduct(b.id)}
                                className="w-4 h-4 rounded border-dark-600 bg-dark-800 text-primary-500 focus:ring-primary-500"
                              />
                              <span className="text-sm text-white">{b.name}</span>
                            </label>
                          ))}
                        </div>
                      </div>
                    )}
                  </>
                )}
              </div>
            )}
          </div>
        </div>

        {error && <p className="text-sm text-red-400 mt-3">{error}</p>}

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} className="px-4 py-2 text-sm text-dark-400 hover:text-white transition-colors">
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !code.trim()}
            className="px-4 py-2 bg-primary-500 text-white text-sm font-medium rounded-lg hover:bg-primary-600 disabled:opacity-50 transition-colors"
          >
            {saving ? 'Creating...' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  );
}
