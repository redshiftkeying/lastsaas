package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"lastsaas/internal/apicounter"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/coupon"
	"github.com/stripe/stripe-go/v82/promotioncode"
)

type PromotionsHandler struct{}

func NewPromotionsHandler() *PromotionsHandler {
	return &PromotionsHandler{}
}

// ListPromotions returns all Stripe promotion codes.
func (h *PromotionsHandler) ListPromotions(w http.ResponseWriter, r *http.Request) {
	params := &stripe.PromotionCodeListParams{}
	params.Limit = stripe.Int64(100)

	iter := promotioncode.List(params)
	apicounter.StripeAPICalls.Add(1)

	type promoResponse struct {
		ID             string  `json:"id"`
		Code           string  `json:"code"`
		Active         bool    `json:"active"`
		CouponID       string  `json:"couponId"`
		CouponName     string  `json:"couponName"`
		PercentOff     float64 `json:"percentOff"`
		AmountOff      int64   `json:"amountOff"`
		Currency       string  `json:"currency"`
		TimesRedeemed  int64   `json:"timesRedeemed"`
		MaxRedemptions int64   `json:"maxRedemptions"`
		Created        int64   `json:"created"`
	}

	var promos []promoResponse
	for iter.Next() {
		pc := iter.PromotionCode()
		p := promoResponse{
			ID:            pc.ID,
			Code:          pc.Code,
			Active:        pc.Active,
			TimesRedeemed: pc.TimesRedeemed,
			Created:       pc.Created,
		}
		if pc.MaxRedemptions > 0 {
			p.MaxRedemptions = pc.MaxRedemptions
		}
		if pc.Coupon != nil {
			p.CouponID = pc.Coupon.ID
			p.CouponName = pc.Coupon.Name
			p.PercentOff = pc.Coupon.PercentOff
			p.AmountOff = pc.Coupon.AmountOff
			p.Currency = string(pc.Coupon.Currency)
		}
		promos = append(promos, p)
	}
	if err := iter.Err(); err != nil {
		log.Printf("Promotions: list error: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to list promotion codes")
		return
	}
	if promos == nil {
		promos = []promoResponse{}
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{"promotions": promos})
}

// CreatePromotion creates a new Stripe coupon + promotion code.
func (h *PromotionsHandler) CreatePromotion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code           string  `json:"code"`
		Name           string  `json:"name"`
		PercentOff     float64 `json:"percentOff"`
		AmountOff      int64   `json:"amountOff"`
		Currency       string  `json:"currency"`
		MaxRedemptions int64   `json:"maxRedemptions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Code == "" {
		respondWithError(w, http.StatusBadRequest, "Promotion code is required")
		return
	}
	if req.PercentOff <= 0 && req.AmountOff <= 0 {
		respondWithError(w, http.StatusBadRequest, "Either percentOff or amountOff is required")
		return
	}

	// Create the coupon first
	couponParams := &stripe.CouponParams{}
	if req.Name != "" {
		couponParams.Name = stripe.String(req.Name)
	} else {
		couponParams.Name = stripe.String(req.Code)
	}
	if req.PercentOff > 0 {
		couponParams.PercentOff = stripe.Float64(req.PercentOff)
	} else {
		couponParams.AmountOff = stripe.Int64(req.AmountOff)
		cur := req.Currency
		if cur == "" {
			cur = "usd"
		}
		couponParams.Currency = stripe.String(cur)
	}

	c, err := coupon.New(couponParams)
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		log.Printf("Promotions: coupon create error: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to create coupon")
		return
	}

	// Create the promotion code
	promoParams := &stripe.PromotionCodeParams{
		Coupon: stripe.String(c.ID),
		Code:   stripe.String(req.Code),
	}
	if req.MaxRedemptions > 0 {
		promoParams.MaxRedemptions = stripe.Int64(req.MaxRedemptions)
	}

	pc, err := promotioncode.New(promoParams)
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		log.Printf("Promotions: promo code create error: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to create promotion code")
		return
	}

	respondWithJSON(w, http.StatusCreated, map[string]interface{}{
		"id":   pc.ID,
		"code": pc.Code,
	})
}

// DeactivatePromotion deactivates a Stripe promotion code.
func (h *PromotionsHandler) DeactivatePromotion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.ID == "" {
		respondWithError(w, http.StatusBadRequest, "Promotion code ID is required")
		return
	}

	_, err := promotioncode.Update(req.ID, &stripe.PromotionCodeParams{
		Active: stripe.Bool(false),
	})
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		log.Printf("Promotions: deactivate error: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to deactivate promotion code")
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}
